// Tests legitimately assert via panic; the strict production
// lints (unwrap/expect/indexing/arithmetic) are relaxed here.
#![allow(clippy::unwrap_used, clippy::expect_used, clippy::indexing_slicing, clippy::arithmetic_side_effects, clippy::panic)]

//! Cross-language golden scenario runners: executes ../golden/scenario*.txt
//! and asserts the emitted traces match the ../golden/expected*.txt files
//! exactly. The Go and Zig ports ship structurally identical runners; see
//! golden/README.md.

use cordis::sync::RefCell;
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use cordis::sync::{BorrowExt as _, Rc};

use cordis::{plugin, start_fn, value, Context, EventOptions, Fiber, FiberState, FnPlugin, Listener, Value};

type Trace = Rc<RefCell<Vec<String>>>;
type FiberMap = Rc<RefCell<HashMap<String, Fiber>>>;
type PluginMap = Rc<RefCell<HashMap<String, Rc<FnPlugin<i32>>>>>;

/// Deferred child plugins started inside a parent plugin's body.
#[derive(Clone)]
struct Spawn {
    name: String,
    deps: Vec<String>,
    config: i32,
}

struct Runner {
    ctx: Context,
    trace: Trace,
    fibers: FiberMap,
    plugins: PluginMap,
    children: HashMap<String, Vec<Spawn>>,
}

fn state_name(state: FiberState) -> &'static str {
    match state {
        FiberState::Pending => "PENDING",
        FiberState::Loading => "LOADING",
        FiberState::Active => "ACTIVE",
        FiberState::Failed => "FAILED",
        FiberState::Disposed => "DISPOSED",
        FiberState::Unloading => "UNLOADING",
    }
}

#[derive(Default)]
struct Params {
    deps: Vec<String>,
    realm: String,
    config: i32,
    lifo: bool,
}

fn parse_params(tokens: &[String]) -> Params {
    let mut p = Params::default();
    for tok in tokens {
        if let Some(rest) = tok.strip_prefix("inject=") {
            p.deps = rest.split(',').filter(|d| !d.is_empty()).map(String::from).collect();
        } else if let Some(rest) = tok.strip_prefix("realm=") {
            p.realm = rest.to_string();
        } else if let Some(rest) = tok.strip_prefix("config=") {
            p.config = rest.parse().expect("config int");
        } else if let Some(rest) = tok.strip_prefix("parent=") {
            p.realm = rest.to_string();
        } else if tok == "lifo" {
            p.lifo = true;
        }
    }
    p
}

fn golden_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../golden")
}

fn read_lines(name: &str) -> Vec<String> {
    let text = fs::read_to_string(golden_dir().join(name)).expect(name);
    text.lines()
        .map(str::trim)
        .filter(|l| !l.is_empty() && !l.starts_with('#'))
        .map(String::from)
        .collect()
}

/// Split a scenario line into op and args.
fn tokenize(line: &str) -> (String, Vec<String>) {
    let mut tokens = line.split_whitespace().map(String::from);
    let op = tokens.next().expect("op");
    (op, tokens.collect())
}

fn compare_trace(trace: &[String], expected: &[String]) {
    assert_eq!(
        trace.len(),
        expected.len(),
        "trace length mismatch\ntrace:\n{}\nexpected:\n{}",
        trace.join("\n"),
        expected.join("\n")
    );
    for (i, (got, want)) in trace.iter().zip(expected.iter()).enumerate() {
        assert_eq!(got, want, "trace divergence at line {}", i + 1);
    }
}

/// Build an uncached plugin whose body traces apply/cleanup and starts any
/// spawned children on its own context.
fn make_plugin(
    name: &str,
    lifo: bool,
    spawns: Vec<Spawn>,
    trace: &Trace,
    fibers: &FiberMap,
) -> FnPlugin<i32> {
    let trace = Rc::clone(trace);
    let fibers = Rc::clone(fibers);
    let name = name.to_string();
    let plugin_name = name.clone();
    plugin(plugin_name.as_str(), move |ctx: &Context, config: &i32| {
        trace.borrow_mut().push(format!("apply {name} config={config}"));
        let labels: Vec<String> = if lifo {
            (1..=3).map(|i| format!("{name}#{i}")).collect()
        } else {
            vec![name.clone()]
        };
        for label in labels {
            let trace = Rc::clone(&trace);
            ctx.attach(move || {
                trace.borrow_mut().push(format!("cleanup {label}"));
            })?;
        }
        for spawn in spawns.iter() {
            let child = make_plugin(&spawn.name, false, Vec::new(), &trace, &fibers);
            let deps: Vec<&str> = spawn.deps.iter().map(String::as_str).collect();
            let child = if deps.is_empty() { child } else { child.inject(&deps) };
            let fiber = start_fn(ctx, &child, spawn.config)?;
            fibers.borrow_mut().insert(spawn.name.clone(), fiber);
        }
        Ok(())
    })
}

impl Runner {
    fn new() -> Runner {
        Runner {
            ctx: Context::new(),
            trace: Rc::new(RefCell::new(Vec::new())),
            fibers: Rc::new(RefCell::new(HashMap::new())),
            plugins: Rc::new(RefCell::new(HashMap::new())),
            children: HashMap::new(),
        }
    }

    /// The cached plugin identity for `name`, built on first use. Caching
    /// pins the registry identity across multiple starts.
    fn plugin(&self, name: &str, params: &Params) -> Rc<FnPlugin<i32>> {
        if let Some(p) = self.plugins.borrow().get(name) {
            return Rc::clone(p);
        }
        let spawns = self.children.get(name).cloned().unwrap_or_default();
        let mut p = make_plugin(name, params.lifo, spawns, &self.trace, &self.fibers);
        let deps: Vec<&str> = params.deps.iter().map(String::as_str).collect();
        if !deps.is_empty() {
            p = p.inject(&deps);
        }
        let p = Rc::new(p);
        self.plugins.borrow_mut().insert(name.to_string(), Rc::clone(&p));
        p
    }

    fn run(&mut self, line: &str) {
        let mut tokens = line.split_whitespace().map(String::from);
        let op = tokens.next().expect("op").clone();
        let args: Vec<String> = tokens.collect();
        let params = parse_params(&args[1.min(args.len())..]);

        match op.as_str() {
            "provide" | "provide-in-realm" => {
                let scope = if op == "provide-in-realm" {
                    self.ctx.isolate_shared(&args[0], &params.realm)
                } else {
                    self.ctx.clone()
                };
                let provider = self.provider_for(&args[0]);
                let fiber = start_fn(&scope, &provider, 0).expect("start provider");
                self.fibers.borrow_mut().insert(format!("provider:{}", args[0]), fiber);
                self.trace.borrow_mut().push(format!("provided {}", args[0]));
            }
            "withdraw" | "withdraw-in-realm" => {
                let fiber = self.fibers.borrow_mut().remove(&format!("provider:{}", args[0])).expect("provider fiber");
                fiber.dispose();
                self.trace.borrow_mut().push(format!("withdrawn {}", args[0]));
            }
            "start" | "start-isolated" => {
                let mut scope = self.ctx.clone();
                if op == "start-isolated" {
                    for dep in &params.deps {
                        scope = scope.isolate_shared(dep, &params.realm);
                    }
                }
                let p = self.plugin(&args[0], &params);
                let fiber = start_fn(&scope, &p, params.config).expect("start");
                self.fibers.borrow_mut().insert(args[0].clone(), fiber);
            }
            "spawn" => {
                let parent = params.realm.clone(); // parent=<name> parsed as realm
                let spawn = Spawn { name: args[0].clone(), deps: params.deps.clone(), config: params.config };
                self.children.entry(parent).or_default().push(spawn);
            }
            "delete" => {
                let p = self.plugin(&args[0], &Params::default());
                self.ctx.registry().delete(&p);
                self.trace.borrow_mut().push(format!("deleted {}", args[0]));
            }
            "expect-registry-size" => {
                let want: usize = args[0].parse().expect("size int");
                let got = self.ctx.registry().size();
                assert_eq!(got, want, "expected registry size {want}, got {got}");
                self.trace.borrow_mut().push(format!("registry-size {want}"));
            }
            "update" => {
                let fiber = self.fibers.borrow().get(&args[0]).cloned().expect("fiber");
                let config: i32 = args[1].parse().expect("config int");
                fiber.update(value(config)).expect("update");
            }
            "restart" => {
                self.fibers.borrow().get(&args[0]).cloned().expect("fiber").restart().expect("restart");
            }
            "dispose" => {
                self.fibers.borrow().get(&args[0]).cloned().expect("fiber").dispose();
                self.trace.borrow_mut().push(format!("disposed {}", args[0]));
            }
            "restart-root" => {
                self.ctx.fiber().dispose();
                self.trace.borrow_mut().push("root-restarted".to_string());
            }
            "expect-state" => {
                let fiber = self.fibers.borrow().get(&args[0]).cloned().expect("fiber");
                let got = state_name(fiber.state());
                assert_eq!(
                    got, args[1],
                    "expected {} {}, trace:\n{}",
                    args[0],
                    args[1],
                    self.trace.borrow().join("\n")
                );
                self.trace.borrow_mut().push(format!("state {} {}", args[0], args[1]));
            }
            other => panic!("unknown op {other}"),
        }
    }

    fn provider_for(&self, service: &str) -> FnPlugin<i32> {
        let service = service.to_string();
        plugin(&format!("provider:{service}"), move |ctx: &Context, _: &i32| {
            ctx.provide_named(&service, value(1i32))?;
            Ok(())
        })
    }
}

fn run_lifecycle_scenario(scenario_file: &str, expected_file: &str) {
    let scenario = read_lines(scenario_file);
    let expected = read_lines(expected_file);

    let mut r = Runner::new();
    for line in &scenario {
        r.run(line);
    }

    let trace = r.trace.borrow();
    compare_trace(&trace, &expected);
}

#[test]
fn golden_scenario() {
    run_lifecycle_scenario("scenario.txt", "expected.txt");
}

#[test]
fn golden_scenario_cascade() {
    run_lifecycle_scenario("scenario-cascade.txt", "expected-cascade.txt");
}

fn event_listener(trace: &Trace, event: &str, who: &str) -> Listener {
    let trace = Rc::clone(trace);
    let prefix = format!("fired {event} {who} payload=");
    Rc::new(move |args: &[Value]| {
        let payload = args[0].downcast_ref::<i32>().copied().unwrap_or(0);
        trace.borrow_mut().push(format!("{prefix}{payload}"));
        None
    })
}

#[test]
fn golden_scenario_events() {
    let scenario = read_lines("scenario-events.txt");
    let expected = read_lines("expected-events.txt");

    let ctx = Context::new();
    let trace: Trace = Rc::new(RefCell::new(Vec::new()));

    for line in &scenario {
        let tokens: Vec<&str> = line.split_whitespace().collect();
        let op = tokens[0];
        let event = tokens[1];
        let mut payload = 0i32;
        let mut realm = String::new();
        for tok in &tokens[2..] {
            if let Some(rest) = tok.strip_prefix("payload=") {
                payload = rest.parse().expect("payload int");
            } else if let Some(rest) = tok.strip_prefix("realm=") {
                realm = rest.to_string();
            }
        }
        match op {
            "on" | "on-isolated" => {
                let (scope, who) = if realm.is_empty() {
                    (ctx.clone(), "root".to_string())
                } else {
                    (ctx.isolate_shared(event, &realm), format!("realm={realm}"))
                };
                scope
                    .on_named(event, event_listener(&trace, event, &who), EventOptions::default())
                    .expect("on");
            }
            "on-global" => {
                ctx.on_named(
                    event,
                    event_listener(&trace, event, "global"),
                    EventOptions { global: true, ..EventOptions::default() },
                )
                .expect("on-global");
            }
            "emit" => ctx.emit_named(event, &[value(payload)]),
            "emit-filtered" => {
                let scope = ctx.isolate_shared(event, &realm);
                let emitter = scope.with_filter(scope.realm_filter(event));
                emitter.emit_named(event, &[value(payload)]);
            }
            other => panic!("unknown op {other}"),
        }
    }

    let got = trace.borrow();
    compare_trace(&got, &expected);
}

#[test]
fn golden_dsl_parse_params() {
    let p = parse_params(&["inject=a,,b".to_string(), "realm=t".to_string(), "config=7".to_string(), "lifo".to_string(), "ignored".to_string()]);
    assert_eq!(p.deps, vec!["a", "b"]);
    assert_eq!(p.realm, "t");
    assert_eq!(p.config, 7);
    assert!(p.lifo);

    let empty = parse_params(&[]);
    assert!(empty.deps.is_empty());
    assert_eq!(empty.config, 0);
    assert!(!empty.lifo);

    // Malformed integers surface as a panic instead of a confusing trace
    // mismatch.
    let result = std::panic::catch_unwind(|| parse_params(&["config=nope".to_string()]));
    assert!(result.is_err(), "malformed config must panic");
}

#[test]
fn golden_dsl_tokenize() {
    let (op, args) = tokenize("  start   worker inject=a config=1 ");
    assert_eq!(op, "start");
    assert_eq!(args, vec!["worker", "inject=a", "config=1"]);
}
