// Benchmarks legitimately assert via panic; the strict production lints
// (unwrap/expect/panic) are relaxed here.
#![allow(clippy::unwrap_used, clippy::expect_used, clippy::panic)]

//! Hot-path benchmarks for the core, mirroring `go/bench_test.go`.
//!
//! `cargo bench` runs this with the release profile; the harness is a plain
//! binary (no dependencies) that reports the best of five runs, because on a
//! shared desktop the best is the only stable statistic.

use std::hint::black_box;
use std::time::Instant;

use cordis::sync::Rc;
use cordis::{plugin, start_fn, value, Context, EventOptions, Next, Value};

struct BenchEvent {
    seq: u64,
}

fn bench(name: &str, iters: u64, mut f: impl FnMut(u64)) {
    for i in 0..1000 {
        f(i);
    }
    let mut best = f64::INFINITY;
    for _ in 0..5 {
        let start = Instant::now();
        for i in 0..iters {
            f(i);
        }
        // iters stays below 100k, exact in f64.
        #[allow(clippy::cast_precision_loss, clippy::as_conversions)]
        let per_op = start.elapsed().as_secs_f64() / iters as f64;
        best = best.min(per_op);
    }
    println!("{name}: {:.0} ns/op (best of 5, {iters} iters)", best * 1e9);
}

fn main() {
    {
        let ctx = Context::new();
        let p = plugin("bench", |_ctx: &Context, config: &u64| {
            black_box(*config);
            Ok(())
        });
        bench("start_dispose", 20_000, |i| {
            let fiber = start_fn(&ctx, &p, i).unwrap();
            fiber.dispose();
        });
    }

    {
        let ctx = Context::new();
        bench("provide_withdraw_get", 20_000, |i| {
            let disposer = ctx.provide(i).unwrap();
            let got = ctx.get::<u64>().unwrap();
            black_box(*got);
            disposer.dispose();
        });
    }

    {
        let ctx = Context::new();
        let disposer = ctx.provide(1_u64).unwrap();
        bench("get", 50_000, |i| {
            black_box(i);
            black_box(ctx.get::<u64>().is_ok());
        });
        disposer.dispose();
    }

    {
        let ctx = Context::new();
        ctx.on_named(
            "bench/event",
            Rc::new(|_: &[Value]| None),
            EventOptions::default(),
        )
        .unwrap();
        bench("event_emit", 50_000, |i| {
            ctx.emit_named("bench/event", &[value(black_box(i))]);
        });
    }

    {
        let ctx = Context::new();
        for _ in 0..5 {
            ctx.on_named(
                "bench/waterfall",
                Rc::new(|args: &[Value]| {
                    let (next_arg, head) = args.split_last()?;
                    let next = next_arg.clone().downcast::<Next>().ok()?;
                    next(head)
                }),
                EventOptions::default(),
            )
            .unwrap();
        }
        let terminal: Next = Rc::new(|_| None);
        bench("waterfall_event", 20_000, |i| {
            ctx.waterfall("bench/waterfall", vec![value(black_box(i))], &terminal);
        });
    }

    {
        let ctx = Context::new();
        ctx.on(
            |e: &BenchEvent| {
                black_box(e.seq);
            },
            EventOptions::default(),
        )
        .unwrap();
        bench("typed_event_dispatch", 50_000, |i| {
            ctx.emit(BenchEvent { seq: black_box(i) });
        });
    }
}
