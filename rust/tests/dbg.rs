#![cfg(feature = "thread-safe")]
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use cordis::{plugin, start_fn, Context};

#[test]
fn concurrent_applies() {
    let ctx = Arc::new(Context::new());
    let started = Arc::new(AtomicUsize::new(0));
    let mut handles = Vec::new();
    for t in 0..4 {
        let ctx = Arc::clone(&ctx);
        let started = Arc::clone(&started);
        handles.push(std::thread::spawn(move || {
            for i in 0..25 {
                let started2 = Arc::clone(&started);
                let name = format!("worker-{t}");
                let plugin = plugin(name.as_str(), move |_ctx: &Context, config: &i32| {
                    started2.fetch_add(1, Ordering::SeqCst);
                    assert_eq!(*config, i);
                    Ok(())
                });
                let fiber = start_fn(&ctx, &plugin, i).expect("start");
                println!("t{t} iter{i} {:?}", fiber.state());
                fiber.dispose();
            }
        }));
    }
    for h in handles { h.join().unwrap(); }
    println!("started={}", started.load(Ordering::SeqCst));
}
