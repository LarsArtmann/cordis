//! The logger service: levels, exporters and a bounded message buffer.
//!
//! Mirrors `LoggerService` upstream (and the Go port's `logger.go`): every
//! context tree owns one service that fans messages out to registered
//! exporters and keeps the most recent messages in memory. The framework's
//! own error channel (failing plugin bodies, panicking cleanups) dispatches
//! into this service at [`Level::Error`]; [`crate::Context::logged_errors`]
//! reads those entries back.
//!
//! Exporters run with no framework borrows held, so they may freely call
//! back into the tree.

use std::collections::HashMap;
use std::time::SystemTime;

use crate::context::{Context, Disposer};
use crate::core::Core;
use crate::sync::{Rc, RefCell};
#[cfg(feature = "thread-safe")]
use crate::sync::BorrowExt as _;

/// A log severity. The variant order is the severity order, mirroring
/// `LoggerLevel` upstream: a message is exported when the target level is
/// greater than or equal to the message level.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum Level {
    /// Failures: a plugin body or cleanup failed.
    Error,
    /// Recoverable problems.
    Warn,
    /// Ordinary operational output.
    Info,
    /// Verbose diagnostics.
    Debug,
}

impl Level {
    /// The canonical name, used as the message kind.
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Error => "error",
            Self::Warn => "warn",
            Self::Info => "info",
            Self::Debug => "debug",
        }
    }
}

/// One typed log argument.
///
/// Rust has no `any` with reflection; arguments
/// carry their rendering class instead, and [`format_message`] applies the
/// printf verbs to it. [`Arg::Json`] is the `%o`/`%O` channel: pass
/// pre-serialized JSON (the crate is dependency free and does not serialize).
#[derive(Debug, Clone, PartialEq)]
pub enum Arg {
    /// Rendered with `%s` and plain display.
    Str(String),
    /// An integer, rendered identically by `%s`, `%d` and `%i`.
    Int(i64),
    /// A float; `%d` truncates it like upstream.
    Float(f64),
    /// An error message; a lone error argument formats through `%s`.
    Error(String),
    /// Pre-serialized JSON, rendered verbatim by `%o`/`%O`.
    Json(String),
}

impl std::fmt::Display for Arg {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Str(s) | Self::Error(s) | Self::Json(s) => write!(f, "{s}"),
            Self::Int(i) => write!(f, "{i}"),
            Self::Float(x) => write!(f, "{x}"),
        }
    }
}

impl From<&str> for Arg {
    fn from(v: &str) -> Self {
        Self::Str(v.to_string())
    }
}

impl From<String> for Arg {
    fn from(v: String) -> Self {
        Self::Str(v)
    }
}

impl From<i64> for Arg {
    fn from(v: i64) -> Self {
        Self::Int(v)
    }
}

impl From<f64> for Arg {
    fn from(v: f64) -> Self {
        Self::Float(v)
    }
}

impl From<crate::Error> for Arg {
    fn from(v: crate::Error) -> Self {
        Self::Error(v.to_string())
    }
}

/// One log record, mirroring the `Message` interface upstream.
#[derive(Debug, Clone, PartialEq)]
pub struct Message {
    /// The tree-wide sequence number.
    pub sn: i64,
    /// When the message was dispatched.
    pub time: SystemTime,
    /// The logger name.
    pub name: String,
    /// The severity name, [`Level::as_str`].
    pub kind: &'static str,
    /// The severity.
    pub level: Level,
    /// The formatted arguments.
    pub args: Vec<Arg>,
}

/// Receives log messages. Implementations must be safe for concurrent use
/// under the `thread-safe` feature.
pub trait Exporter: crate::sync::Shared {
    /// Deliver one message.
    fn export(&self, message: &Message);
}

/// The logger intercept value, installed with [`Context::intercept`]:
///
/// ```
/// use cordis::{Context, Level, LoggerIntercept};
///
/// let ctx = Context::new();
/// let scoped = ctx.intercept("logger", cordis::value(LoggerIntercept {
///     name: Some("worker".to_string()),
///     level: Some(Level::Warn),
/// }));
/// assert_eq!(scoped.logger(None).name(), "worker");
/// ```
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct LoggerIntercept {
    /// The logger name override.
    pub name: Option<String>,
    /// The level override: messages above it are dropped before dispatch.
    pub level: Option<Level>,
}

enum Sink {
    /// The built-in bounded buffer.
    Buffer,
    /// A user exporter.
    Exporter(Rc<dyn Exporter>),
}

struct ExporterEntry {
    sink: Sink,
    levels: Option<HashMap<String, Level>>,
}

/// The per-tree logging facility. Owned by the core; reach it through
/// [`Context::logger`], [`Context::add_exporter`] and friends.
#[derive(Default)]
pub struct LoggerService {
    sn_message: i64,
    sn_exporter: i64,
    exporters: Vec<(i64, ExporterEntry)>,
    buffer: Vec<Message>,
    buffer_size: usize,
}

impl LoggerService {
    /// A service with the default buffer exporter (1000 messages) attached.
    pub fn new() -> Self {
        let mut service = Self {
            buffer_size: 1000,
            ..Self::default()
        };
        service.sn_exporter = service.sn_exporter.wrapping_add(1);
        service.exporters.push((
            service.sn_exporter,
            ExporterEntry {
                sink: Sink::Buffer,
                levels: None,
            },
        ));
        service
    }

    /// The most recent messages, oldest first.
    #[must_use]
    pub fn buffer(&self) -> &[Message] {
        &self.buffer
    }

    /// Resize the message buffer, truncating the oldest entries.
    pub fn set_buffer_size(&mut self, size: usize) {
        self.buffer_size = size;
        let overflow = self.buffer.len().saturating_sub(size);
        if overflow > 0 {
            self.buffer.drain(..overflow);
        }
    }

    /// Register an exporter and return a Disposer removing it. `levels`
    /// maps logger names to their minimum exported level; the `"default"`
    /// key applies to every other name. No map means `info`.
    pub fn add_exporter(
        &mut self,
        exporter: Rc<dyn Exporter>,
        levels: Option<HashMap<String, Level>>,
    ) -> i64 {
        self.sn_exporter = self.sn_exporter.wrapping_add(1);
        let id = self.sn_exporter;
        self.exporters.push((
            id,
            ExporterEntry {
                sink: Sink::Exporter(exporter),
                levels,
            },
        ));
        id
    }

    /// Remove every exporter, including the built-in buffer.
    pub fn clear_exporters(&mut self) {
        self.exporters.clear();
    }

    fn remove_exporter(&mut self, id: i64) {
        self.exporters.retain(|(candidate, _)| *candidate != id);
    }

    /// Assign the sequence number, append to the buffer and select the
    /// exporters whose level target admits the message. The callers run the
    /// selected exporters after every framework borrow is released.
    fn prepare(&mut self, name: &str, level: Level, args: Vec<Arg>) -> (Message, Vec<Rc<dyn Exporter>>) {
        self.sn_message = self.sn_message.wrapping_add(1);
        let message = Message {
            sn: self.sn_message,
            time: SystemTime::now(),
            name: name.to_string(),
            kind: level.as_str(),
            level,
            args,
        };
        if self.buffer.len() >= self.buffer_size && self.buffer_size > 0 {
            self.buffer.remove(0);
        }
        if self.buffer_size > 0 {
            self.buffer.push(message.clone());
        }
        let exporters = self
            .exporters
            .iter()
            .filter_map(|(_, entry)| {
                let target = entry.levels.as_ref().map_or(Level::Info, |levels| {
                    levels.get(name).or_else(|| levels.get("default")).copied().unwrap_or(Level::Info)
                });
                if target < level {
                    return None;
                }
                match &entry.sink {
                    Sink::Exporter(exporter) => Some(Rc::clone(exporter)),
                    Sink::Buffer => None,
                }
            })
            .collect();
        (message, exporters)
    }
}

/// A named, leveled logging handle, mirroring `Logger` upstream. Create one
/// with [`Context::logger`].
pub struct Logger {
    core: Rc<RefCell<Core>>,
    name: String,
    level: Option<Level>,
}

impl Logger {
    /// The logger name.
    #[must_use]
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Log at [`Level::Error`].
    pub fn error(&self, args: &[Arg]) {
        self.log(Level::Error, args);
    }

    /// Log at [`Level::Warn`].
    pub fn warn(&self, args: &[Arg]) {
        self.log(Level::Warn, args);
    }

    /// Log at [`Level::Info`].
    pub fn info(&self, args: &[Arg]) {
        self.log(Level::Info, args);
    }

    /// Log at [`Level::Debug`].
    pub fn debug(&self, args: &[Arg]) {
        self.log(Level::Debug, args);
    }

    /// Dispatch at `level` unless the handle's level intercept rejects it.
    pub fn log(&self, level: Level, args: &[Arg]) {
        if self.level.is_some_and(|l| l < level) {
            return;
        }
        dispatch(&self.core, &self.name, level, args.to_vec());
    }
}

/// Dispatch one message: prepare under the core borrow (sequence number,
/// buffer, exporter selection), then run the user exporters with no borrow
/// held.
fn dispatch(core: &Rc<RefCell<Core>>, name: &str, level: Level, args: Vec<Arg>) {
    let (message, exporters) = {
        let mut c = core.borrow_mut();
        c.logger.prepare(name, level, args)
    };
    for exporter in &exporters {
        exporter.export(&message);
    }
}

/// The framework's internal error channel, mirroring `ctx.logger.error`
/// upstream. A missing name falls back to `"root"`.
pub fn log_error(core: &Rc<RefCell<Core>>, name: &str, message: &str) {
    let name = if name.is_empty() { "root" } else { name };
    dispatch(core, name, Level::Error, vec![Arg::Error(message.to_string())]);
}

/// Render a message to a single string, mirroring the format pipeline
/// upstream.
///
/// The first argument is a printf style format string supporting
/// `%s`, `%d`, `%i`, `%f`, `%o`, `%O` and `%%` verbs, and remaining
/// arguments are appended separated by spaces. A lone error argument
/// formats through `%s`; any other non-string first argument through `%o`.
#[must_use]
pub fn format_message(message: &Message) -> String {
    let mut args = message.args.clone();
    let prepend = match args.first() {
        Some(Arg::Error(text)) => {
            let text = text.clone();
            if let Some(first) = args.first_mut() {
                *first = Arg::Str(text);
            }
            Some("%s")
        }
        Some(Arg::Str(_)) => None,
        Some(_) => Some("%o"),
        None => return String::new(),
    };
    if let Some(verb) = prepend {
        args.insert(0, Arg::Str(verb.to_string()));
    }
    let Some((Arg::Str(format), rest)) = args.split_first() else {
        return String::new();
    };

    let mut out = String::new();
    let mut arg_index = 0usize;
    let mut chars = format.chars();
    while let Some(c) = chars.next() {
        if c != '%' {
            out.push(c);
            continue;
        }
        let Some(verb) = chars.next() else {
            out.push(c);
            continue;
        };
        if verb == '%' {
            out.push('%');
            continue;
        }
        let Some(value) = rest.get(arg_index) else {
            out.push('%');
            out.push(verb);
            continue;
        };
        arg_index = arg_index.saturating_add(1);
        match verb {
            's' => out.push_str(&value.to_string()),
            'd' | 'i' => out.push_str(&truncate_number(value)),
            'f' => out.push_str(&render_float(value)),
            'o' | 'O' => match value {
                Arg::Json(json) => out.push_str(json),
                other => out.push_str(&other.to_string()),
            },
            'c' | 'C' => {}
            _ => {
                out.push('%');
                out.push(verb);
            }
        }
    }
    for value in rest.iter().skip(arg_index) {
        out.push(' ');
        out.push_str(&value.to_string());
    }
    out
}

/// `%d`/`%i`: integers pass through, floats truncate, numeric strings
/// parse then truncate, anything else renders as `0` like upstream. The
/// float truncation is the upstream behavior, hence the deliberate casts.
#[allow(clippy::cast_possible_truncation, clippy::as_conversions)]
fn truncate_number(value: &Arg) -> String {
    match value {
        Arg::Int(i) => i.to_string(),
        Arg::Float(x) => format!("{}", *x as i64),
        Arg::Str(s) => {
            let parsed: f64 = s.parse().unwrap_or(0.0);
            format!("{}", parsed as i64)
        }
        Arg::Error(text) | Arg::Json(text) => {
            let parsed: f64 = text.parse().unwrap_or(0.0);
            format!("{}", parsed as i64)
        }
    }
}

/// `%f`: floats render losslessly, integers as themselves.
fn render_float(value: &Arg) -> String {
    match value {
        Arg::Float(x) => format!("{x}"),
        Arg::Int(i) => i.to_string(),
        Arg::Str(s) | Arg::Error(s) | Arg::Json(s) => s.clone(),
    }
}

/// An exporter writing formatted messages to a [`std::io::Write`] sink,
/// mirroring the `logger-console` package upstream.
///
/// Writes are best effort:
/// a logger cannot report its own write failure through logging.
pub struct ConsoleExporter<W> {
    out: RefCell<W>,
}

impl<W: std::io::Write + crate::sync::Shared> ConsoleExporter<W> {
    /// Wrap a writer. [`ConsoleExporter::stderr`] is the common choice.
    #[must_use]
    pub const fn new(out: W) -> Self {
        Self { out: RefCell::new(out) }
    }
}

impl ConsoleExporter<std::io::Stderr> {
    /// The standard error console exporter.
    #[must_use]
    pub fn stderr() -> Self {
        Self::new(std::io::stderr())
    }
}

impl<W: std::io::Write + crate::sync::Shared> Exporter for ConsoleExporter<W> {
    fn export(&self, message: &Message) {
        let line = format!("[{}] {}: {}\n", message.kind, message.name, format_message(message));
        // Deliberate discard: a logger cannot report its own write failure
        // through logging.
        let _ = self.out.borrow_mut().write_all(line.as_bytes());
    }
}

impl Context {
    /// A logger for this context. Name resolution mirrors upstream: an
    /// explicit non-empty argument wins, then the nearest
    /// [`LoggerIntercept`] installed through [`Context::intercept`], then
    /// the name of the fiber owning the context.
    #[must_use]
    pub fn logger(&self, name: Option<&str>) -> Logger {
        let resolved = name.filter(|n| !n.is_empty()).map(std::string::ToString::to_string);
        let (resolved, level) = if let Some(intercept) = self.intercepted("logger")
            && let Some(li) = intercept.downcast_ref::<LoggerIntercept>()
        {
            (resolved.or_else(|| li.name.clone()), li.level)
        } else {
            (resolved, None)
        };
        Logger {
            core: Rc::clone(&self.core),
            name: resolved.unwrap_or_else(|| self.fiber().name()),
            level,
        }
    }

    /// Register a log exporter on this tree and return a Disposer removing
    /// it. `levels` maps logger names to their minimum exported level; the
    /// `"default"` key applies to every other name. `None` exports
    /// everything at `info` or above.
    pub fn add_exporter(
        &self,
        exporter: Rc<dyn Exporter>,
        levels: Option<HashMap<String, Level>>,
    ) -> Disposer {
        let id = {
            let mut core = self.core.borrow_mut();
            core.logger.add_exporter(exporter, levels)
        };
        Disposer::new({
            let core = Rc::clone(&self.core);
            move || {
                core.borrow_mut().logger.remove_exporter(id);
            }
        })
    }

    /// Remove every log exporter, including the built-in buffer.
    pub fn clear_exporters(&self) {
        self.core.borrow_mut().logger.clear_exporters();
    }

    /// A copy of the most recent log messages, oldest first.
    #[must_use]
    pub fn logger_buffer(&self) -> Vec<Message> {
        self.core.borrow().logger.buffer().to_vec()
    }

    /// Resize the message buffer, truncating the oldest entries.
    pub fn set_logger_buffer_size(&self, size: usize) {
        self.core.borrow_mut().logger.set_buffer_size(size);
    }
}
