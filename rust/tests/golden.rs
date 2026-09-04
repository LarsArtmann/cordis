//! Cross-language golden scenario runner: executes ../golden/scenario.txt
//! and asserts the emitted trace matches ../golden/expected.txt exactly.
//! The Go and Zig ports ship structurally identical runners; see
//! golden/README.md.

use std::cell::RefCell;
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use std::rc::Rc;

use cordis::{plugin, start_fn, Context, Fiber, FiberState};

type Trace = Rc<RefCell<Vec<String>>>;

struct Runner {
    trace: Trace,
    fibers: HashMap<String, Fiber>,
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

fn parse_params(tokens: &[&str]) -> Params {
    let mut p = Params::default();
    for tok in tokens {
        if let Some(rest) = tok.strip_prefix("inject=") {
            p.deps = rest.split(',').filter(|d| !d.is_empty()).map(String::from).collect();
        } else if let Some(rest) = tok.strip_prefix("realm=") {
            p.realm = rest.to_string();
        } else if let Some(rest) = tok.strip_prefix("config=") {
            p.config = rest.parse().expect("config int");
        } else if *tok == "lifo" {
            p.lifo = true;
        }
    }
    p
}

impl Runner {
    fn log(&self, line: String) {
        self.trace.borrow_mut().push(line);
    }

    fn build_plugin(&self, name: &str, params: &Params) -> cordis::FnPlugin<i32> {
        let trace = Rc::clone(&self.trace);
        let name = name.to_string();
        let plugin_name = name.clone();
        let lifo = params.lifo;
        let p = plugin(plugin_name.as_str(), move |ctx: &Context, config: &i32| {
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
            Ok(())
        });
        let deps: Vec<&str> = params.deps.iter().map(String::as_str).collect();
        if deps.is_empty() {
            p
        } else {
            p.inject(&deps)
        }
    }

    fn build_provider(&self, service: &str) -> cordis::FnPlugin<i32> {
        let service = service.to_string();
        plugin(&format!("provider:{service}"), move |ctx: &Context, _: &i32| {
            ctx.provide_named(&service, cordis::value(1i32))?;
            Ok(())
        })
    }
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

#[test]
fn golden_scenario() {
    let scenario = read_lines("scenario.txt");
    let expected = read_lines("expected.txt");

    let ctx = Context::new();
    let mut r = Runner {
        trace: Rc::new(RefCell::new(Vec::new())),
        fibers: HashMap::new(),
    };

    for line in &scenario {
        let tokens: Vec<&str> = line.split_whitespace().collect();
        let op = tokens[0];
        let args = &tokens[1..];
        match op {
            "provide" | "provide-in-realm" => {
                let params = parse_params(args);
                let scope = if op == "provide-in-realm" {
                    ctx.isolate_shared(args[0], &params.realm)
                } else {
                    ctx.clone()
                };
                let provider = r.build_provider(args[0]);
                let fiber = start_fn(&scope, &provider, 0).expect("start provider");
                r.fibers.insert(format!("provider:{}", args[0]), fiber);
                r.log(format!("provided {}", args[0]));
            }
            "withdraw" | "withdraw-in-realm" => {
                let fiber = r.fibers.remove(&format!("provider:{}", args[0])).expect("provider fiber");
                fiber.dispose();
                r.log(format!("withdrawn {}", args[0]));
            }
            "start" | "start-isolated" => {
                let params = parse_params(args);
                let mut scope = ctx.clone();
                if op == "start-isolated" {
                    for dep in &params.deps {
                        scope = scope.isolate_shared(dep, &params.realm);
                    }
                }
                let p = r.build_plugin(args[0], &params);
                let fiber = start_fn(&scope, &p, params.config).expect("start");
                r.fibers.insert(args[0].to_string(), fiber);
            }
            "update" => {
                let fiber = r.fibers.get(args[0]).expect("fiber");
                let config: i32 = args[1].parse().expect("config int");
                fiber.update(cordis::value(config)).expect("update");
            }
            "restart" => {
                r.fibers.get(args[0]).expect("fiber").restart().expect("restart");
            }
            "dispose" => {
                r.fibers.get(args[0]).expect("fiber").dispose();
                r.log(format!("disposed {}", args[0]));
            }
            "restart-root" => {
                ctx.fiber().dispose();
                r.log("root-restarted".to_string());
            }
            "expect-state" => {
                let fiber = r.fibers.get(args[0]).expect("fiber");
                let got = state_name(fiber.state());
                assert_eq!(
                    got, args[1],
                    "expected {} {}, trace:\n{}",
                    args[0],
                    args[1],
                    r.trace.borrow().join("\n")
                );
                r.log(format!("state {} {}", args[0], args[1]));
            }
            other => panic!("unknown op {other}"),
        }
    }

    let trace = r.trace.borrow();
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
