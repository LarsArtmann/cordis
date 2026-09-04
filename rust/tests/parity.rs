//! Parity tests mirroring the Go port's suite, which mirrors the TypeScript
//! core test suite.

use cordis::sync::RefCell;
use cordis::sync::{BorrowExt as _, Rc};

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
    let result = ctx.waterfall("test", vec![value(1)], terminal).unwrap();
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
        move |ctx: &Context, _: &()| {
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
        move |ctx: &Context, _: &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            Ok(())
        }
    });
    let mid = plugin("mid", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, _: &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            start_fn(ctx, &inner, ())?;
            Ok(())
        }
    });
    let outer = plugin("outer", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, _: &()| {
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
        move |ctx: &Context, _: &()| {
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
        move |ctx: &Context, _: &()| {
            ctx.on_named("custom-event", counting_listener(&calls), opts())?;
            Err(Error::Validation("boom".to_string()))
        }
    });
    let healthy = plugin("healthy", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, _: &()| {
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
