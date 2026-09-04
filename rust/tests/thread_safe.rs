//! Stress tests for the `thread-safe` feature: multiple threads drive one
//! context tree through the public API. The framework's drain queue keeps
//! fiber transitions single-threaded internally, so the tree stays
//! consistent without user-visible locks.
#![cfg(feature = "thread-safe")]

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use cordis::{plugin, start_fn, value, Context, EventOptions};

#[test]
fn threads_share_one_tree() {
    let ctx = Arc::new(Context::new());
    let started = Arc::new(AtomicUsize::new(0));
    let received = Arc::new(AtomicUsize::new(0));

    let sink = Arc::new(AtomicUsize::new(0));
    let sink2 = Arc::clone(&sink);
    ctx.on_named(
        "tick",
        Arc::new(move |_args: &[cordis::Value]| {
            sink2.fetch_add(1, Ordering::SeqCst);
            None
        }),
        EventOptions::default(),
    )
    .expect("on");

    let mut handles = Vec::new();
    for t in 0..4 {
        let ctx = Arc::clone(&ctx);
        let started = Arc::clone(&started);
        let received = Arc::clone(&received);
        #[allow(unused_variables)]
        let t = t;
        handles.push(std::thread::spawn(move || {
            for i in 0..25 {
                let name = format!("worker-{t}");
                let plugin = plugin(name.as_str(), {
                    let started = Arc::clone(&started);
                    move |_ctx: &Context, config: &i32| {
                        started.fetch_add(1, Ordering::SeqCst);
                        assert_eq!(*config, i);
                        Ok(())
                    }
                });
                let fiber = start_fn(&ctx, &plugin, i).expect("start");
                ctx.emit_named("tick", &[value(i)]);
                received.fetch_add(1, Ordering::SeqCst);
                fiber.dispose();
            }
        }));
    }
    for handle in handles {
        handle.join().expect("thread");
    }

    // Starts and disposals coalesce inside the drain queue: a fiber whose
    // disposal is queued before its deferred drain never applies. The
    // event count is exact; the apply count is bounded above.
    assert_eq!(received.load(Ordering::SeqCst), 4 * 25);
    assert_eq!(sink.load(Ordering::SeqCst), 4 * 25);
    let applied_before = started.load(Ordering::SeqCst);
    assert!(applied_before <= 4 * 25);

    // One more fiber per thread applies and settles synchronously.
    let mut handles = Vec::new();
    for t in 0..4 {
        let ctx = Arc::clone(&ctx);
        let started = Arc::clone(&started);
        handles.push(std::thread::spawn(move || {
            let name = format!("final-{t}");
            let plugin = plugin(name.as_str(), move |_ctx: &Context, config: &i32| {
                started.fetch_add(1, Ordering::SeqCst);
                assert_eq!(*config, 7);
                Ok(())
            });
            start_fn(&ctx, &plugin, 7).expect("start");
        }));
    }
    for handle in handles {
        handle.join().expect("thread");
    }
    assert_eq!(started.load(Ordering::SeqCst), applied_before + 4);
}

#[test]
fn services_and_events_race_safely() {
    let ctx = Arc::new(Context::new());
    let hits = Arc::new(AtomicUsize::new(0));

    let mut handles = Vec::new();
    for t in 0..4 {
        let ctx = Arc::clone(&ctx);
        let hits = Arc::clone(&hits);
        handles.push(std::thread::spawn(move || {
            let scope = ctx.isolate_shared("shared", &format!("realm-{t}"));
            let provider_plugin = plugin("provider", move |c: &Context, _: &i32| {
                c.provide_named("dep", value(1i32))?;
                Ok(())
            });
            let consumer_plugin = plugin("consumer", {
                let hits = Arc::clone(&hits);
                move |c: &Context, _: &i32| {
                    if c.get_named("dep").is_some() {
                        hits.fetch_add(1, Ordering::SeqCst);
                    }
                    Ok(())
                }
            })
            .inject(&["dep"]);
            for _ in 0..10 {
                let provider = start_fn(&scope, &provider_plugin, 0).expect("provider");
                let _ = start_fn(&scope, &consumer_plugin, 0).expect("consumer");
                provider.dispose();
            }
        }));
    }
    for handle in handles {
        handle.join().expect("thread");
    }
    assert!(hits.load(Ordering::SeqCst) > 0);
}
