//! Example smoke app for the Rust port: a typed `Greeter` service, a typed
//! `Greeted` event, and the synchronous drain contract (the listener has
//! already run when `emit` returns). Run with `cargo run --example greeter`.

use cordis::{Context, EventOptions};

/// Provided under its type identity; consumers look it up with
/// [`Context::get`].
struct Greeter {
    name: String,
}

impl Greeter {
    fn greet(&self, who: &str) -> String {
        format!("Hello, {who}, from {}!", self.name)
    }
}

/// The typed event emitted after a greeting renders.
struct Greeted {
    line: String,
}

fn main() -> Result<(), cordis::Error> {
    let ctx = Context::new();

    ctx.provide(Greeter {
        name: "cordis".to_string(),
    })?;
    ctx.on(
        |event: &Greeted| println!("{}", event.line),
        EventOptions::default(),
    )?;
    let greeter = ctx.get::<Greeter>()?;
    ctx.emit(Greeted {
        line: greeter.greet("world"),
    });

    Ok(())
}
