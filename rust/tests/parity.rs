// Tests legitimately assert via panic; the strict production
// lints (unwrap/expect/indexing/arithmetic) are relaxed here.
#![allow(clippy::unwrap_used, clippy::expect_used, clippy::indexing_slicing, clippy::arithmetic_side_effects, clippy::panic, clippy::too_many_lines, clippy::as_conversions)]

//! Parity tests mirroring the Go port's suite, which mirrors the TypeScript
//! core test suite.

use cordis::sync::RefCell;
#[cfg(feature = "thread-safe")]
use cordis::sync::BorrowExt as _;
use cordis::sync::Rc;

use cordis::{plugin, start_fn, value, Context, Error, EventOptions, FiberState, Next, Value};

fn opts() -> EventOptions {
    EventOptions::default()
}

type Counter = Rc<RefCell<i32>>;
type TestListener = cordis::Listener;

fn counting_listener(calls: &Counter) -> TestListener {
    let calls = Rc::clone(calls);
    Rc::new(move |_| {
        *calls.borrow_mut() += 1;
        None
    })
}

#[test]
fn on_emit_dispose() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    let disposer = ctx.on_named("test", counting_listener(&calls), opts()).unwrap();
    ctx.emit_named("test", &[]);
    assert_eq!(*calls.borrow(), 1);
    disposer.dispose();
    ctx.emit_named("test", &[]);
    assert_eq!(*calls.borrow(), 1);
}

#[test]
fn once_fires_exactly_once() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    let _once = ctx.once_named("test", counting_listener(&calls), opts()).unwrap();
    ctx.emit_named("test", &[]);
    ctx.emit_named("test", &[]);
    assert_eq!(*calls.borrow(), 1);
}

#[test]
fn emit_order_with_prepend() {
    let ctx = Context::new();
    let seq = Rc::new(RefCell::new(Vec::new()));
    let register = |v: i32, o: EventOptions| {
        let seq = Rc::clone(&seq);
        ctx.on_named(
            "test",
            Rc::new(move |_| {
                seq.borrow_mut().push(v);
                None
            }),
            o,
        )
        .unwrap()
    };
    let _a = register(1, opts());
    let _b = register(2, opts());
    let _c = register(
        0,
        EventOptions {
            prepend: true,
            global: false,
        },
    );
    ctx.emit_named("test", &[]);
    assert_eq!(*seq.borrow(), vec![0, 1, 2]);
}

#[test]
fn bail_returns_first_result() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    for result in [None, Some(value("bailed")), Some(value("unreachable"))] {
        let calls = Rc::clone(&calls);
        ctx.on_named(
            "test",
            Rc::new(move |_| {
                *calls.borrow_mut() += 1;
                result.clone()
            }),
            opts(),
        )
        .unwrap();
    }
    let result = ctx.bail("test", &[]).unwrap();
    assert_eq!(*result.downcast::<&str>().unwrap(), "bailed");
    assert_eq!(*calls.borrow(), 2);
}

#[test]
fn waterfall_composes_and_short_circuits() {
    let ctx = Context::new();
    let add_next: TestListener = Rc::new(|args| {
        let v = *args[0].clone().downcast::<i32>().unwrap();
        let next = args[1].clone().downcast::<Next>().unwrap();
        let result = next(&[value(v)]).unwrap();
        Some(value(v + *result.downcast::<i32>().unwrap()))
    });
    ctx.on_named("test", Rc::clone(&add_next), opts()).unwrap();
    ctx.on_named("test", add_next, opts()).unwrap();
    let terminal: Next = Rc::new(|_| Some(value(2)));
    let result = ctx.waterfall("test", vec![value(1)], &terminal).unwrap();
    assert_eq!(*result.downcast::<i32>().unwrap(), 4);
}

#[test]
fn parallel_aggregates_errors() {
    let ctx = Context::new();
    ctx.on_named("test", Rc::new(|_| Some(value("err-one".to_string()))), opts())
        .unwrap();
    ctx.on_named("test", Rc::new(|_| Some(value("err-two".to_string()))), opts())
        .unwrap();
    let err = ctx.parallel("test", &[]).unwrap_err();
    let text = err.to_string();
    assert!(text.contains("err-one"), "missing err-one in {text}");
    assert!(text.contains("err-two"), "missing err-two in {text}");
}

#[test]
fn event_filter() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    ctx.on_named("test", counting_listener(&calls), opts()).unwrap();
    ctx.emit_named("test", &[]);
    assert_eq!(*calls.borrow(), 1);

    let rejecting = ctx.with_filter(Rc::new(|_| false));
    rejecting.emit_named("test", &[]);
    assert_eq!(*calls.borrow(), 1);

    // Global listeners bypass filters.
    ctx.on_named(
        "global-test",
        counting_listener(&calls),
        EventOptions {
            prepend: false,
            global: true,
        },
    )
    .unwrap();
    rejecting.emit_named("global-test", &[]);
    assert_eq!(*calls.borrow(), 2);
}

#[test]
fn effect_rolls_back_lifo() {
    let ctx = Context::new();
    let seq = Rc::new(RefCell::new(Vec::new()));
    ctx.effect("outer", {
        let seq = Rc::clone(&seq);
        move |ctx| {
            ctx.attach({
                let seq = Rc::clone(&seq);
                move || seq.borrow_mut().push(1)
            })?;
            ctx.effect("inner", {
                let seq = Rc::clone(&seq);
                move |ctx| {
                    ctx.attach(move || seq.borrow_mut().push(2))?;
                    Ok(())
                }
            })?;
            ctx.attach(move || seq.borrow_mut().push(3))?;
            Ok(())
        }
    })
    .unwrap();
    ctx.fiber().dispose(); // root fiber: restart, rolling back every effect
    assert_eq!(*seq.borrow(), vec![3, 2, 1]);
}

#[test]
fn effect_error_rolls_back_partial_registrations() {
    let ctx = Context::new();
    let seq = Rc::new(RefCell::new(0));
    let result = ctx.effect("faulty", {
        let seq = Rc::clone(&seq);
        move |ctx| {
            ctx.attach(move || *seq.borrow_mut() += 1)?;
            Err(Error::Validation("boom".to_string()))
        }
    });
    assert!(result.is_err());
    assert_eq!(*seq.borrow(), 1);
}

#[test]
fn effect_on_inactive_context_fails() {
    let ctx = Context::new();
    let captured: Rc<RefCell<Option<Context>>> = Rc::new(RefCell::new(None));
    let p = plugin("p", {
        let captured = Rc::clone(&captured);
        move |ctx: &Context, (): &()| {
            captured.borrow_mut().replace(ctx.clone());
            Ok(())
        }
    });
    let fiber = start_fn(&ctx, &p, ()).unwrap();
    fiber.dispose();
    let inner = captured.borrow_mut().take().unwrap();
    assert_eq!(
        inner.effect("x", |_| Ok(())).unwrap_err(),
        Error::InactiveEffect
    );
    assert_eq!(
        inner.on_named("x", Rc::new(|_| None), opts()).unwrap_err(),
        Error::InactiveEffect
    );
    assert_eq!(start_fn(&inner, &p, ()).unwrap_err(), Error::InactiveEffect);
}

#[test]
fn plugin_lifecycle() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    let p = plugin("greeter", {
        let calls = Rc::clone(&calls);
        move |_ctx: &Context, cfg: &String| {
            assert_eq!(cfg, "hello");
            *calls.borrow_mut() += 1;
            Ok(())
        }
    });
    let fiber = start_fn(&ctx, &p, "hello".to_string()).unwrap();
    assert_eq!(*calls.borrow(), 1);
    assert_eq!(fiber.state(), FiberState::Active);
    assert_eq!(fiber.name(), "greeter");

    fiber.restart().unwrap();
    assert_eq!(*calls.borrow(), 2);

    fiber.dispose();
    assert_eq!(fiber.state(), FiberState::Disposed);
    assert_eq!(fiber.uid(), -1);
    fiber.dispose(); // idempotent
}

#[test]
fn nested_plugins_and_registry() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));

    let inner = plugin("inner", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, (): &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            Ok(())
        }
    });
    let mid = plugin("mid", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, (): &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            start_fn(ctx, &inner, ())?;
            Ok(())
        }
    });
    let outer = plugin("outer", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, (): &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            start_fn(ctx, &mid, ())?;
            Ok(())
        }
    });

    let fiber = start_fn(&ctx, &outer, ()).unwrap();
    assert_eq!(ctx.registry().size(), 3);
    ctx.emit_named("custom-event", &[]);
    assert_eq!(*calls.borrow(), 3);

    fiber.dispose();
    assert_eq!(ctx.registry().size(), 0);
    ctx.emit_named("custom-event", &[]);
    assert_eq!(*calls.borrow(), 3);
}

#[test]
fn registry_delete_restores_snapshot() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    let p = plugin("p", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, (): &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            Ok(())
        }
    });
    start_fn(&ctx, &p, ()).unwrap();
    ctx.emit_named("custom-event", &[]);
    assert_eq!(*calls.borrow(), 1);

    ctx.registry().delete(&p);
    assert!(!ctx.registry().has(&p));
    ctx.emit_named("custom-event", &[]);
    assert_eq!(*calls.borrow(), 1);

    start_fn(&ctx, &p, ()).unwrap();
    ctx.emit_named("custom-event", &[]);
    assert_eq!(*calls.borrow(), 2);
}

#[test]
fn plugin_error_rolls_back_partial_effects() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    let faulty = plugin("faulty", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, (): &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            Err(Error::Validation("boom".to_string()))
        }
    });
    let healthy = plugin("healthy", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, (): &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            Ok(())
        }
    });

    let faulty_fiber = start_fn(&ctx, &faulty, ()).unwrap();
    start_fn(&ctx, &healthy, ()).unwrap();

    assert_eq!(faulty_fiber.state(), FiberState::Failed);
    assert_eq!(ctx.logged_errors().len(), 1);
    ctx.emit_named("custom-event", &[]);
    assert_eq!(*calls.borrow(), 1);
}

#[test]
fn update_reinvokes_with_new_config() {
    let ctx = Context::new();
    let msgs = Rc::new(RefCell::new(Vec::new()));
    let p = plugin("p", {
        let msgs = Rc::clone(&msgs);
        move |_ctx: &Context, cfg: &String| {
            msgs.borrow_mut().push(cfg.clone());
            Ok(())
        }
    });
    let fiber = start_fn(&ctx, &p, "hello".to_string()).unwrap();
    fiber.update(value("world".to_string())).unwrap();
    assert_eq!(*msgs.borrow(), vec!["hello".to_string(), "world".to_string()]);
    assert_eq!(fiber.state(), FiberState::Active);
}

#[test]
fn inject_reactivity() {
    let ctx = Context::new();
    let seq: Rc<RefCell<Vec<&str>>> = Rc::new(RefCell::new(Vec::new()));
    let fiber = ctx
        .inject(&["foo"], {
            let seq = Rc::clone(&seq);
            move |ctx: &Context| {
                seq.borrow_mut().push("apply");
                ctx.attach({
                    let seq = Rc::clone(&seq);
                    move || seq.borrow_mut().push("cleanup")
                })?;
                Ok(())
            }
        })
        .unwrap();
    assert_eq!(fiber.state(), FiberState::Pending);

    let disposer = ctx.provide_named("foo", value(1)).unwrap();
    assert_eq!(fiber.state(), FiberState::Active);
    assert_eq!(*seq.borrow(), vec!["apply"]);

    disposer.dispose();
    assert_eq!(fiber.state(), FiberState::Pending);
    assert_eq!(*seq.borrow(), vec!["apply", "cleanup"]);

    ctx.provide_named("foo", value(2)).unwrap();
    assert_eq!(fiber.state(), FiberState::Active);
    assert_eq!(*seq.borrow(), vec!["apply", "cleanup", "apply"]);
}

#[test]
fn inject_after_clone_returns_plugin_shared() {
    let ctx = Context::new();
    let p = plugin("pinned", |_: &Context, (): &()| Ok(()));
    let clone = p.clone();
    let Err(err) = clone.inject(&["dep"]) else {
        panic!("inject after clone must fail")
    };
    assert_eq!(
        err,
        Error::PluginShared {
            name: "pinned".to_string()
        }
    );
    let p = p.inject(&["dep"]).expect("inject before any clone");
    let fiber = start_fn(&ctx, &p, ()).unwrap();
    assert_eq!(fiber.state(), FiberState::Pending);
    ctx.provide_named("dep", value(1)).unwrap();
    assert_eq!(fiber.state(), FiberState::Active);
}

#[test]
fn provide_get_and_duplicate_detection() {
    let ctx = Context::new();
    assert!(ctx.get_named("foo").is_none());
    let disposer = ctx.provide_named("foo", value(42)).unwrap();
    assert_eq!(*ctx.get_named("foo").unwrap().downcast::<i32>().unwrap(), 42);
    assert!(ctx.provide_named("foo", value(43)).is_err());
    disposer.dispose();
    assert!(ctx.get_named("foo").is_none());
}

#[test]
fn isolation_realms() {
    let ctx = Context::new();
    let iso1 = ctx.isolate("foo");
    let iso2 = ctx.isolate("foo");

    let calls = Rc::new(RefCell::new(0));
    for scope in [&ctx, &iso1, &iso2] {
        scope
            .inject(&["foo"], {
                let calls = Rc::clone(&calls);
                move |_ctx| {
                    *calls.borrow_mut() += 1;
                    Ok(())
                }
            })
            .unwrap();
    }

    ctx.provide_named("foo", value(100)).unwrap();
    assert_eq!(*calls.borrow(), 1);
    assert!(iso1.get_named("foo").is_none());

    iso1.provide_named("foo", value(200)).unwrap();
    assert_eq!(*calls.borrow(), 2);
    assert!(iso2.get_named("foo").is_none());
    assert_eq!(*ctx.get_named("foo").unwrap().downcast::<i32>().unwrap(), 100);
}

#[test]
fn isolation_shared_label() {
    let ctx = Context::new();
    let iso1 = ctx.isolate_shared("foo", "shared");
    let iso2 = ctx.isolate_shared("foo", "shared");

    let calls = Rc::new(RefCell::new(0));
    for scope in [&iso1, &iso2] {
        scope
            .inject(&["foo"], {
                let calls = Rc::clone(&calls);
                move |_ctx| {
                    *calls.borrow_mut() += 1;
                    Ok(())
                }
            })
            .unwrap();
    }

    let disposer = iso1.provide_named("foo", value(200)).unwrap();
    assert_eq!(*calls.borrow(), 2);
    assert_eq!(*iso2.get_named("foo").unwrap().downcast::<i32>().unwrap(), 200);

    disposer.dispose();
    assert!(iso1.get_named("foo").is_none());
    assert!(iso2.get_named("foo").is_none());
}

#[test]
fn isolation_shared_labels_are_collision_free() {
    let ctx = Context::new();
    // With the previous "{name}\0{label}" synthetic key these two distinct
    // pairs collapsed into one realm.
    let a = ctx.isolate_shared("foo\0bar", "baz");
    let b = ctx.isolate_shared("foo", "bar\0baz");

    a.provide_named("foo", value(1)).unwrap();
    assert!(b.get_named("foo").is_none(), "distinct (name, label) pairs must denote distinct realms");

    // Equal pairs still share one realm.
    let a2 = ctx.isolate_shared("foo\0bar", "baz");
    assert!(a2.get_named("foo").is_some(), "equal pairs must share the realm");
}

#[test]
fn realm_filtered_events() {
    let ctx = Context::new();
    let isolated = ctx.isolate("foo");
    let root_calls = Rc::new(RefCell::new(0));
    let iso_calls = Rc::new(RefCell::new(0));
    ctx.on_named("custom-event", counting_listener(&root_calls), opts())
        .unwrap();
    isolated
        .on_named("custom-event", counting_listener(&iso_calls), opts())
        .unwrap();

    let emitter = isolated.with_filter(isolated.realm_filter("foo"));
    emitter.emit_named("custom-event", &[]);
    assert_eq!(*root_calls.borrow(), 0);
    assert_eq!(*iso_calls.borrow(), 1);

    ctx.emit_named("custom-event", &[]);
    assert_eq!(*root_calls.borrow(), 1);
    assert_eq!(*iso_calls.borrow(), 2);
}

#[test]
fn status_events_emission_order() {
    use cordis::{FiberState as FS, StatusChange, EVENT_STATUS};

    let ctx = Context::new();
    let seq: Rc<RefCell<Vec<String>>> = Rc::new(RefCell::new(Vec::new()));
    {
        let seq = Rc::clone(&seq);
        ctx.on_named(
            EVENT_STATUS,
            Rc::new(move |args: &[Value]| {
                let change = args[0].downcast_ref::<StatusChange>().unwrap();
                seq.borrow_mut().push(format!(
                    "{}:{:?}-> {:?}",
                    change.name, change.old, change.new
                ));
                None
            }),
            opts(),
        )
        .unwrap();
    }

    let p = plugin("svc", |_ctx: &Context, (): &()| Ok(()));
    let fiber = start_fn(&ctx, &p, ()).unwrap();
    let started = vec![
        "svc:Pending-> Loading".to_string(),
        "svc:Loading-> Active".to_string(),
    ];
    assert_eq!(*seq.borrow(), started);

    // A restart passes through unloading and loading again.
    seq.borrow_mut().clear();
    fiber.restart().unwrap();
    let restarted = vec![
        "svc:Active-> Unloading".to_string(),
        "svc:Unloading-> Loading".to_string(),
        "svc:Loading-> Active".to_string(),
    ];
    assert_eq!(*seq.borrow(), restarted);

    // Disposal unwinds through unloading into disposed; the name stays
    // with the fiber even though the runtime is already gone.
    seq.borrow_mut().clear();
    fiber.dispose();
    let disposed = vec![
        "svc:Active-> Unloading".to_string(),
        "svc:Unloading-> Disposed".to_string(),
    ];
    assert_eq!(*seq.borrow(), disposed);
    assert_eq!(fiber.state(), FS::Disposed);
}

#[test]
fn snapshot_restore_roundtrip() {
    let ctx = Context::new();
    let starts = Rc::new(RefCell::new(0));
    let p = plugin("svc", {
        let starts = Rc::clone(&starts);
        move |ctx: &Context, conf: &u32| {
            *starts.borrow_mut() += 1;
            ctx.provide_named("port", value(*conf))?;
            Ok(())
        }
    });

    let fiber = start_fn(&ctx, &p, 8080_u32).unwrap();
    let snapshot = ctx.snapshot();
    assert_eq!(snapshot.runtimes.len(), 1);
    assert_eq!(snapshot.runtimes[0].name, "svc");
    assert_eq!(snapshot.runtimes[0].fibers.len(), 1);

    // Delete rolls the registry back; restore brings the runtime back
    // with its config and it serves again.
    ctx.registry().delete(&p);
    assert_eq!(ctx.snapshot().runtimes.len(), 0);
    ctx.restore(&snapshot);
    assert_eq!(*starts.borrow(), 2);
    assert_eq!(
        *ctx.get_named("port").unwrap().downcast::<u32>().unwrap(),
        8080
    );

    // Restoring an older snapshot disposes runtimes that appeared since.
    let after = ctx.snapshot();
    let extra = plugin("extra", |_ctx: &Context, (): &()| Ok(()));
    start_fn(&ctx, &extra, ()).unwrap();
    assert_eq!(ctx.snapshot().runtimes.len(), 2);
    ctx.restore(&after);
    let now = ctx.snapshot();
    assert_eq!(now.runtimes.len(), 1);
    assert_eq!(now.runtimes[0].name, "svc");
    assert_eq!(now.runtimes[0].fibers[0].state, FiberState::Active);
    drop(fiber);
}

#[test]
fn plugin_events_fire_on_create_and_dispose() {
    use cordis::{EVENT_PLUGIN, Fiber};

    let ctx = Context::new();
    let seen: Rc<RefCell<Vec<(String, FiberState, i64)>>> = Rc::new(RefCell::new(Vec::new()));
    {
        let seen = Rc::clone(&seen);
        ctx.on_named(
            EVENT_PLUGIN,
            Rc::new(move |args: &[Value]| {
                let fiber = args.first().and_then(|v| v.downcast_ref::<Fiber>())?;
                seen.borrow_mut().push((fiber.name(), fiber.state(), fiber.uid()));
                None
            }),
            opts(),
        )
        .unwrap();
    }

    let p = plugin("watched", |_ctx: &Context, (): &()| Ok(()));
    let fiber = start_fn(&ctx, &p, ()).unwrap();
    // Creation fires once, before the first transition: the payload fiber is
    // still pending and already carries its identity.
    let uid = fiber.uid();
    assert_eq!(
        *seen.borrow(),
        vec![("watched".to_string(), FiberState::Pending, uid)]
    );

    seen.borrow_mut().clear();
    fiber.dispose();
    // Disposal fires once, before the effects roll back: the payload fiber
    // is still active, keeps its name, and its uid is not yet retired to -1.
    assert_eq!(
        *seen.borrow(),
        vec![("watched".to_string(), FiberState::Active, uid)]
    );
    assert_eq!(fiber.state(), FiberState::Disposed);
}

#[test]
fn update_waterfall_rewrites_config() {
    use cordis::{EVENT_UPDATE, Fiber};

    let ctx = Context::new();
    let observed: Rc<RefCell<Vec<(String, i32, bool)>>> = Rc::new(RefCell::new(Vec::new()));
    {
        let observed = Rc::clone(&observed);
        ctx.on_named(
            EVENT_UPDATE,
            Rc::new(move |args: &[Value]| {
                let fiber = args.first().and_then(|v| v.downcast_ref::<Fiber>())?;
                let config = args.get(1).and_then(|v| v.downcast_ref::<i32>()).copied()?;
                let no_save = args.get(2).and_then(|v| v.downcast_ref::<bool>()).copied()?;
                observed.borrow_mut().push((fiber.name(), config, no_save));
                let next = args.get(3).and_then(|v| v.clone().downcast::<Next>().ok())?;
                next(&[args.first().cloned()?, value(config * 10), value(no_save)])
            }),
            opts(),
        )
        .unwrap();
    }

    let applied: Rc<RefCell<Vec<i32>>> = Rc::new(RefCell::new(Vec::new()));
    let p = plugin("cfg", {
        let applied = Rc::clone(&applied);
        move |_ctx: &Context, cfg: &i32| {
            applied.borrow_mut().push(*cfg);
            Ok(())
        }
    });
    let fiber = start_fn(&ctx, &p, 1_i32).unwrap();
    fiber.update(value(7_i32)).unwrap();

    // The listener saw the raw update (fiber, config, no_save) and rewrote
    // the config tenfold; the plugin body re-ran with the rewritten value.
    assert_eq!(*observed.borrow(), vec![("cfg".to_string(), 7, false)]);
    assert_eq!(*applied.borrow(), vec![1, 70]);
    assert_eq!(fiber.state(), FiberState::Active);
}

#[test]
fn update_waterfall_veto_stops_the_restart() {
    use cordis::EVENT_UPDATE;

    let ctx = Context::new();
    ctx.on_named(EVENT_UPDATE, Rc::new(|_| Some(value(()))), opts())
        .unwrap();

    let applied: Rc<RefCell<Vec<i32>>> = Rc::new(RefCell::new(Vec::new()));
    let p = plugin("vetoed", {
        let applied = Rc::clone(&applied);
        move |_ctx: &Context, cfg: &i32| {
            applied.borrow_mut().push(*cfg);
            Ok(())
        }
    });
    let fiber = start_fn(&ctx, &p, 1_i32).unwrap();
    fiber.update(value(2_i32)).unwrap();

    // The listener ended the chain without calling next, so the config and
    // the running body are untouched.
    assert_eq!(*applied.borrow(), vec![1]);
    assert_eq!(fiber.state(), FiberState::Active);
}

#[test]
fn root_fiber_rejects_update() {
    let ctx = Context::new();
    assert_eq!(ctx.fiber().update(value(1_i32)), Err(Error::RootUpdate));
    // The root is untouched: still active with its original identity.
    assert_eq!(ctx.fiber().state(), FiberState::Active);
    assert_eq!(ctx.fiber().uid(), 0);
}

#[test]
fn root_restart_rolls_back_and_stays_active() {
    let ctx = Context::new();
    let cleanups: Rc<RefCell<Vec<&str>>> = Rc::new(RefCell::new(Vec::new()));
    ctx.attach({
        let cleanups = Rc::clone(&cleanups);
        move || cleanups.borrow_mut().push("root-effect")
    })
    .unwrap();

    ctx.fiber().restart().unwrap();
    // The root scope rolled back, but the fiber itself survived with its
    // identity intact (uid 0, active), mirroring upstream's restartRoot.
    assert_eq!(*cleanups.borrow(), vec!["root-effect"]);
    assert_eq!(ctx.fiber().state(), FiberState::Active);
    assert_eq!(ctx.fiber().uid(), 0);

    // The fresh bag accepts new effects.
    ctx.attach(|| {}).unwrap();
}

#[test]
fn root_dispose_emits_no_plugin_event() {
    use cordis::EVENT_PLUGIN;

    // A plugin fiber brackets its life with internal/plugin: one event on
    // start, one on disposal.
    let ctx = Context::new();
    let events = Rc::new(RefCell::new(0));
    let _listener = ctx
        .on_named(EVENT_PLUGIN, counting_listener(&events), opts())
        .unwrap();
    let p = plugin("p", |_ctx: &Context, (): &()| Ok(()));
    let fiber = start_fn(&ctx, &p, ()).unwrap();
    assert_eq!(*events.borrow(), 1);

    // Root disposal is a rollback-and-restart of the root scope: the
    // root-scoped plugin cascades (firing its own disposal event), but the
    // root itself owns no plugin runtime and never fires the event.
    ctx.fiber().dispose();
    assert_eq!(*events.borrow(), 2);
    assert_eq!(fiber.state(), FiberState::Disposed);

    // With no plugin fiber in play, root disposal fires nothing at all.
    let root = Context::new();
    let root_events = Rc::new(RefCell::new(0));
    let _listener = root
        .on_named(EVENT_PLUGIN, counting_listener(&root_events), opts())
        .unwrap();
    root.fiber().dispose();
    assert_eq!(*root_events.borrow(), 0);
}

#[test]
fn root_restart_drains_root_scoped_listeners() {
    use cordis::{StatusChange, EVENT_STATUS};

    let ctx = Context::new();
    let disposed_seen = Rc::new(RefCell::new(0));
    {
        let disposed_seen = Rc::clone(&disposed_seen);
        ctx.on_named(
            EVENT_STATUS,
            Rc::new(move |args: &[Value]| {
                if let Some(change) = args.first().and_then(|v| v.downcast_ref::<StatusChange>())
                    && change.new == FiberState::Disposed
                {
                    *disposed_seen.borrow_mut() += 1;
                }
                None
            }),
            opts(),
        )
        .unwrap();
    }

    ctx.fiber().restart().unwrap();
    // The restart rolled the root scope back, taking the listener with it,
    // and never passed through settle_state: no disposed event anywhere.
    assert_eq!(*disposed_seen.borrow(), 0);

    // A fresh listener observes normal plugin transitions again.
    let seen = Rc::new(RefCell::new(0));
    {
        let seen = Rc::clone(&seen);
        ctx.on_named(
            EVENT_STATUS,
            Rc::new(move |_| {
                *seen.borrow_mut() += 1;
                None
            }),
            opts(),
        )
        .unwrap();
    }
    let p = plugin("svc", |_ctx: &Context, (): &()| Ok(()));
    start_fn(&ctx, &p, ()).unwrap();
    assert_eq!(*seen.borrow(), 2);
}

#[test]
fn root_fiber_birth_emits_no_status() {
    use cordis::{StatusChange, EVENT_STATUS};

    let ctx = Context::new();
    let seen: Rc<RefCell<Vec<StatusChange>>> = Rc::new(RefCell::new(Vec::new()));
    {
        let seen = Rc::clone(&seen);
        ctx.on_named(
            EVENT_STATUS,
            Rc::new(move |args: &[Value]| {
                if let Some(change) = args.first().and_then(|v| v.downcast_ref::<StatusChange>()) {
                    seen.borrow_mut().push(change.clone());
                }
                None
            }),
            opts(),
        )
        .unwrap();
    }

    // The root fiber is born active: a listener registered on a fresh
    // context has received no status event, because creation is not a
    // state transition.
    assert!(seen.borrow().is_empty());
    assert_eq!(ctx.fiber().state(), FiberState::Active);
    assert_eq!(ctx.fiber().uid(), 0);

    // Real plugin transitions fire the usual pending -> loading -> active
    // pair, so the empty log above pins the birth contract, not dead wiring.
    let p = plugin("svc", |_ctx: &Context, (): &()| Ok(()));
    start_fn(&ctx, &p, ()).unwrap();
    assert_eq!(seen.borrow().len(), 2, "pending -> loading -> active");
}

#[test]
fn interception_events() {
    use cordis::{GetResult, ListenerRef, SetOutcome, EVENT_DISPATCH, EVENT_GET, EVENT_LISTENER, EVENT_SET};

    let ctx = Context::new();

    // EVENT_GET: supply a fallback for a missing service.
    {
        let listener: cordis::Listener = Rc::new(|args: &[Value]| {
            // Defensive: the dispatch probe below re-emits this event with
            // no waterfall arguments.
            let next = args.get(2)?;
            let next: Next = Rc::clone(next.downcast_ref::<Next>().unwrap());
            let result = next(&[value(GetResult {
                value: Some(value("fallback".to_string())),
                ok: true,
            })])
            .unwrap();
            Some(result)
        });
        ctx.on_named(EVENT_GET, listener, opts()).unwrap();
    }
    let fallback = ctx.get_named("missing").unwrap();
    assert_eq!(*fallback.downcast::<String>().unwrap(), "fallback");

    // Existing services bypass the interception.
    ctx.provide_named("real", value(1_i64)).unwrap();
    let real = ctx.get_named("real").unwrap();
    assert_eq!(*real.downcast::<i64>().unwrap(), 1);

    // EVENT_SET: veto a registration and observe a good one.
    let vetoed = Rc::new(RefCell::new(false));
    let seen = Rc::new(RefCell::new(false));
    {
        let vetoed = Rc::clone(&vetoed);
        let seen = Rc::clone(&seen);
        let listener: cordis::Listener = Rc::new(move |args: &[Value]| {
            let name = args[0].downcast_ref::<String>().unwrap().clone();
            if name == "forbidden" {
                *vetoed.borrow_mut() = true;
                return Some(Rc::new(Error::Validation("set rejected".to_string())));
            }
            let next: Next = Rc::clone(args[2].downcast_ref::<Next>().unwrap());
            let result = next(&[Rc::clone(&args[0]), Rc::clone(&args[1])]);
            *seen.borrow_mut() = true;
            result
        });
        ctx.on_named(EVENT_SET, listener, opts()).unwrap();
    }
    assert!(ctx.provide_named("forbidden", value(1_i64)).is_err());
    assert!(*vetoed.borrow());
    assert!(!*seen.borrow());
    ctx.provide_named("allowed", value(2_i64)).unwrap();
    assert!(*seen.borrow());

    // EVENT_LISTENER: replace a registration with a custom disposer.
    {
        let listener: cordis::Listener = Rc::new(|args: &[Value]| {
            let name = args[0].downcast_ref::<String>().unwrap();
            if name == "hooked" {
                let outcome: Rc<RefCell<SetOutcome>> =
                    Rc::new(RefCell::new(SetOutcome { entry: None }));
                outcome.borrow_mut().entry = Some(Ok(cordis::Disposer::new(|| {})));
                return Some(outcome);
            }
            None
        });
        ctx.on_named(EVENT_LISTENER, listener, opts()).unwrap();
    }
    let replaced = ctx
        .on_named(
            "hooked",
            Rc::new(|_: &[Value]| None),
            opts(),
        )
        .unwrap();
    replaced.dispose(); // the replacement disposer, idempotent

    // The intercepted registration never happened: "hooked" has no
    // listeners, so a fresh (unintercepted) listener on another name is
    // needed to prove dispatch still works.
    ctx.emit_named("hooked", &[]);
    let calls = Rc::new(RefCell::new(0));
    ctx.on_named(
        "counter",
        {
            let calls = Rc::clone(&calls);
            Rc::new(move |_| {
                *calls.borrow_mut() += 1;
                None
            })
        },
        opts(),
    )
    .unwrap();
    ctx.emit_named("counter", &[]);
    assert_eq!(*calls.borrow(), 1);

    // EVENT_DISPATCH: observe non-internal emissions only.
    let modes = Rc::new(RefCell::new(Vec::<String>::new()));
    {
        let modes = Rc::clone(&modes);
        let listener: cordis::Listener = Rc::new(move |args: &[Value]| {
            let mode = args[0].downcast_ref::<String>().unwrap().clone();
            let name = args[1].downcast_ref::<String>().unwrap().clone();
            modes.borrow_mut().push(format!("{mode},{name}"));
            None
        });
        ctx.on_named(EVENT_DISPATCH, listener, opts()).unwrap();
    }
    ctx.emit_named("user-event", &[value(1_i64)]);
    ctx.emit_named(EVENT_GET, &[]);
    assert_eq!(*modes.borrow(), vec!["emit,user-event".to_string()]);

    // The listener payload of EVENT_LISTENER carries the listener.
    let got_listener = Rc::new(RefCell::new(false));
    {
        let got = Rc::clone(&got_listener);
        let listener: cordis::Listener = Rc::new(move |args: &[Value]| {
            if args[0].downcast_ref::<String>().map(String::as_str) == Some("probe") {
                let _payload = args[1].downcast_ref::<ListenerRef>().unwrap();
                *got.borrow_mut() = true;
            }
            None
        });
        ctx.on_named(EVENT_LISTENER, listener, opts()).unwrap();
    }
    ctx.on_named("probe", Rc::new(|_: &[Value]| None), opts()).unwrap();
    assert!(*got_listener.borrow());
}

#[test]
fn logger_buffer_exporters_and_levels() {
    use cordis::{Arg, Exporter, Level, Message};

    struct Collector {
        lines: RefCell<Vec<String>>,
    }
    impl Exporter for Collector {
        fn export(&self, message: &Message) {
            self.lines.borrow_mut().push(cordis::format_message(message));
        }
    }

    let ctx = Context::new();
    ctx.set_logger_buffer_size(2);
    let logger = ctx.logger(Some("test"));
    logger.info(&[Arg::from("one")]);
    logger.info(&[Arg::from("two")]);
    logger.info(&[Arg::from("three")]);
    let texts: Vec<String> = ctx
        .logger_buffer()
        .iter()
        .map(cordis::format_message)
        .collect();
    assert_eq!(texts, vec!["two".to_string(), "three".to_string()]);

    let collector = Rc::new(Collector { lines: RefCell::new(Vec::new()) });
    let remove = ctx.add_exporter(Rc::clone(&collector) as Rc<dyn Exporter>, None);
    let mut levels = std::collections::HashMap::new();
    levels.insert("quiet".to_string(), Level::Error);
    let quiet_only = ctx.add_exporter(
        Rc::new(Collector { lines: RefCell::new(Vec::new()) }) as Rc<dyn Exporter>,
        Some(levels),
    );
    quiet_only.dispose();
    ctx.logger(Some("test")).info(&[Arg::from("hello")]);
    assert_eq!(*collector.lines.borrow(), vec!["hello".to_string()]);
    remove.dispose();
    ctx.logger(Some("test")).info(&[Arg::from("world")]);
    assert_eq!(*collector.lines.borrow(), vec!["hello".to_string()]);
}

#[test]
fn logger_name_resolution_and_intercept_level() {
    use cordis::{Level, LoggerIntercept};

    let ctx = Context::new();
    assert_eq!(ctx.logger(None).name(), "root");
    assert_eq!(ctx.logger(Some("custom")).name(), "custom");

    let scoped = ctx.intercept(
        "logger",
        value(LoggerIntercept {
            name: Some("scoped".to_string()),
            level: Some(Level::Warn),
        }),
    );
    assert_eq!(scoped.logger(None).name(), "scoped");
    // Explicit names win over intercepts.
    assert_eq!(scoped.logger(Some("explicit")).name(), "explicit");

    // The intercept level gates dispatch: info is dropped, warn passes.
    ctx.set_logger_buffer_size(100);
    scoped.logger(None).info(&[cordis::Arg::from("hidden")]);
    scoped.logger(None).warn(&[cordis::Arg::from("shown")]);
    let texts: Vec<String> = ctx
        .logger_buffer()
        .iter()
        .map(cordis::format_message)
        .collect();
    assert_eq!(texts, vec!["shown".to_string()]);

    // Fiber names resolve last.
    let p = plugin("worker", |_: &Context, (): &()| Ok(()));
    let fiber = start_fn(&ctx, &p, ()).unwrap();
    assert_eq!(fiber.context().logger(None).name(), "worker");
}

#[test]
fn logger_format_pipeline() {
    use cordis::{format_message, Arg, Level, Message};

    let render = |args: Vec<Arg>| {
        format_message(&Message {
            sn: 1,
            time: std::time::SystemTime::UNIX_EPOCH,
            name: "test".to_string(),
            kind: "info",
            level: Level::Info,
            args,
        })
    };
    assert_eq!(render(vec![Arg::from("hello")]), "hello");
    assert_eq!(
        render(vec![Arg::from("value: %d"), Arg::Float(42.7)]),
        "value: 42"
    );
    assert_eq!(
        render(vec![Arg::from("%s %s"), Arg::from("a"), Arg::from("b")]),
        "a b"
    );
    assert_eq!(
        render(vec![Arg::from("%o"), Arg::Json(r#"{"a":1}"#.to_string())]),
        r#"{"a":1}"#
    );
    assert_eq!(render(vec![Arg::from("100%%")]), "100%");
    assert_eq!(
        render(vec![Arg::from("tail"), Arg::Int(1), Arg::from("two")]),
        "tail 1 two"
    );
    assert_eq!(render(vec![Arg::from("es%o %d %f")]), "es%o %d %f");
}

#[test]
fn framework_errors_flow_through_the_logger() {
    let ctx = Context::new();
    let faulty = plugin("faulty", |_: &Context, (): &()| {
        Err(Error::Validation("boom".to_string()))
    });
    let faulty_fiber = start_fn(&ctx, &faulty, ()).unwrap();
    assert_eq!(faulty_fiber.state(), FiberState::Failed);
    let errors = ctx.logged_errors();
    assert_eq!(errors.len(), 1);
    assert_eq!(errors[0], "<faulty> invalid config: boom");
}

#[test]
fn attach_labeled_exposes_labels_in_introspection() {
    let ctx = Context::new();
    let ran = Rc::new(RefCell::new(0));
    {
        let ran = Rc::clone(&ran);
        let _effect = ctx
            .effect("test", move |ctx: &Context| {
                let ran = Rc::clone(&ran);
                ctx.attach_labeled("my-cleanup", move || {
                    *ran.borrow_mut() += 1;
                })?;
                Ok(())
            })
            .unwrap();
    }
    let effects = ctx.fiber().effects();
    assert_eq!(effects.len(), 1);
    assert_eq!(effects[0].label, "test");
    assert_eq!(effects[0].children.len(), 1);
    assert_eq!(effects[0].children[0].label, "my-cleanup");
    assert_eq!(effects[0].children[0].children, Vec::new());

    // Default label for the unlabeled attach.
    let _plain = ctx.attach(|| {}).unwrap();
    assert_eq!(ctx.fiber().effects()[1].label, "ctx.attach()");
}
