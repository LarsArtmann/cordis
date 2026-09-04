//! Tests for the native API forms: typed services, typed events, RAII
//! guards and the Plugin trait.

use cordis::sync::RefCell;
use cordis::sync::{BorrowExt as _, Rc};

use cordis::{
    event_name, plugin, plugin_type_id, service_name, start, start_fn, Context, EventOptions,
    FiberState, Guard, Plugin,
};

#[derive(Default)]
struct Database {
    dsn: String,
}

#[derive(Default)]
struct Cache;

#[derive(Debug, PartialEq, Clone)]
struct UserCreated {
    id: i32,
}

#[derive(Debug, PartialEq, Clone)]
struct UserDeleted {
    id: i32,
}

fn opts() -> EventOptions {
    EventOptions::default()
}

#[test]
fn typed_service_round_trip() {
    let ctx = Context::new();
    let db = Database { dsn: "postgres://localhost".to_string() };

    assert!(ctx.try_get::<Database>().is_none());
    assert!(ctx.get::<Database>().is_err());

    ctx.provide(db).unwrap();
    assert_eq!(ctx.get::<Database>().unwrap().dsn, "postgres://localhost");
    assert!(ctx.try_get::<Cache>().is_none());
}

#[test]
fn typed_service_duplicate_and_withdrawal() {
    let ctx = Context::new();
    let disposer = ctx.provide(Database::default()).unwrap();
    assert!(ctx.provide(Database::default()).is_err());
    disposer.dispose();
    assert!(ctx.try_get::<Database>().is_none());
    ctx.provide(Database::default()).unwrap();
}

#[test]
fn typed_service_inject_reactivity() {
    let ctx = Context::new();
    let activations = Rc::new(RefCell::new(0));
    let consumer = plugin("consumer", {
        let activations = Rc::clone(&activations);
        move |ctx: &Context, _: &()| {
            *activations.borrow_mut() += 1;
            ctx.get::<Database>()?;
            Ok(())
        }
    })
    .inject(&[service_name::<Database>()]);

    let fiber = start_fn(&ctx, &consumer, ()).unwrap();
    assert_eq!(fiber.state(), FiberState::Pending);

    let disposer = ctx.provide(Database::default()).unwrap();
    assert_eq!(fiber.state(), FiberState::Active);
    assert_eq!(*activations.borrow(), 1);

    disposer.dispose();
    assert_eq!(fiber.state(), FiberState::Pending);
    assert_eq!(*activations.borrow(), 1);
}

#[test]
fn typed_service_isolation() {
    let ctx = Context::new();
    let isolated = ctx.isolate(service_name::<Database>());

    ctx.provide(Database { dsn: "root".into() }).unwrap();
    isolated
        .provide(Database { dsn: "isolated".into() })
        .unwrap();

    assert_eq!(ctx.get::<Database>().unwrap().dsn, "root");
    assert_eq!(isolated.get::<Database>().unwrap().dsn, "isolated");
}

#[test]
fn typed_events_dispatch_by_type() {
    let ctx = Context::new();
    let received: Rc<RefCell<Vec<String>>> = Rc::new(RefCell::new(Vec::new()));

    ctx.on(
        {
            let received = Rc::clone(&received);
            move |e: &UserCreated| received.borrow_mut().push(format!("created {}", e.id))
        },
        opts(),
    )
    .unwrap();
    ctx.on(
        {
            let received = Rc::clone(&received);
            move |e: &UserDeleted| received.borrow_mut().push(format!("deleted {}", e.id))
        },
        opts(),
    )
    .unwrap();

    ctx.emit(UserCreated { id: 1 });
    ctx.emit(UserDeleted { id: 2 });
    ctx.emit(UserCreated { id: 3 });

    assert_eq!(
        *received.borrow(),
        vec!["created 1".to_string(), "deleted 2".to_string(), "created 3".to_string()]
    );
}

#[test]
fn typed_event_once() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    ctx.once(
        {
            let calls = Rc::clone(&calls);
            move |_: &UserCreated| *calls.borrow_mut() += 1
        },
        opts(),
    )
    .unwrap();
    ctx.emit(UserCreated { id: 1 });
    ctx.emit(UserCreated { id: 2 });
    assert_eq!(*calls.borrow(), 1);
}

#[test]
fn typed_event_rolls_back_with_fiber() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    let p = plugin("listener", {
        let calls = Rc::clone(&calls);
        move |ctx: &Context, _: &()| {
            ctx.on(
                {
                    let calls = Rc::clone(&calls);
                    move |_: &UserCreated| *calls.borrow_mut() += 1
                },
                opts(),
            )?;
            Ok(())
        }
    });
    let fiber = start_fn(&ctx, &p, ()).unwrap();
    ctx.emit(UserCreated { id: 1 });
    assert_eq!(*calls.borrow(), 1);

    fiber.dispose();
    ctx.emit(UserCreated { id: 2 });
    assert_eq!(*calls.borrow(), 1);
}

#[test]
fn event_name_resolves_from_type() {
    assert_eq!(event_name::<UserCreated>(), service_name::<UserCreated>());
    assert_ne!(event_name::<UserCreated>(), event_name::<UserDeleted>());
}

struct Worker;

struct WorkerConfig {
    name: String,
}

impl Plugin for Worker {
    type Config = WorkerConfig;

    fn name(&self) -> &str {
        "worker"
    }

    fn inject(&self) -> Vec<String> {
        vec![service_name::<Database>().to_string()]
    }

    fn apply(&self, ctx: &Context, config: &WorkerConfig) -> cordis::Result<()> {
        let db = ctx.get::<Database>()?;
        assert_eq!(db.dsn, "live");
        let name = config.name.clone();
        ctx.attach(move || {
            let _ = name;
        })?;
        Ok(())
    }
}

#[test]
fn plugin_trait_start_and_registry() {
    let ctx = Context::new();

    let fiber = start(&ctx, Worker, WorkerConfig { name: "w1".into() }).unwrap();
    assert_eq!(fiber.state(), FiberState::Pending);
    assert_eq!(fiber.name(), "worker");

    ctx.provide(Database { dsn: "live".into() }).unwrap();
    assert_eq!(fiber.state(), FiberState::Active);
    assert!(ctx.registry().has_id(plugin_type_id::<Worker>()));

    // Starting the same plugin type again creates a second fiber of ONE runtime.
    let second = start(&ctx, Worker, WorkerConfig { name: "w2".into() }).unwrap();
    assert_eq!(second.state(), FiberState::Active);
    assert_eq!(ctx.registry().size(), 1);

    ctx.registry().delete_id(plugin_type_id::<Worker>());
    assert!(!ctx.registry().has_id(plugin_type_id::<Worker>()));
    assert_eq!(fiber.state(), FiberState::Disposed);
    assert_eq!(second.state(), FiberState::Disposed);
}

struct Recorder;

#[derive(Clone)]
struct RecorderConfig {
    labels: Rc<RefCell<Vec<String>>>,
}

impl Plugin for Recorder {
    type Config = RecorderConfig;

    fn name(&self) -> &str {
        "recorder"
    }

    fn apply(&self, ctx: &Context, config: &RecorderConfig) -> cordis::Result<()> {
        config.labels.borrow_mut().push("apply".to_string());
        ctx.attach({
            let labels = Rc::clone(&config.labels);
            move || labels.borrow_mut().push("cleanup".to_string())
        })?;
        Ok(())
    }
}

#[test]
fn plugin_trait_update_restarts_with_new_config() {
    let ctx = Context::new();
    let labels: Rc<RefCell<Vec<String>>> = Rc::new(RefCell::new(Vec::new()));

    let fiber = start(
        &ctx,
        Recorder,
        RecorderConfig { labels: Rc::clone(&labels) },
    )
    .unwrap();
    assert_eq!(*labels.borrow(), vec!["apply".to_string()]);

    fiber
        .update(cordis::value(RecorderConfig { labels: Rc::clone(&labels) }))
        .unwrap();
    assert_eq!(*labels.borrow(), vec!["apply".to_string(), "cleanup".to_string(), "apply".to_string()]);
    assert_eq!(fiber.state(), FiberState::Active);
}

#[test]
fn guard_disposes_on_drop() {
    let ctx = Context::new();
    {
        let _scoped = ctx.provide(Database::default()).map(Guard::from).unwrap();
        assert!(ctx.try_get::<Database>().is_some());
    }
    assert!(ctx.try_get::<Database>().is_none());
}

#[test]
fn guard_detach_keeps_registration() {
    let ctx = Context::new();
    let calls = Rc::new(RefCell::new(0));
    {
        let disposer = ctx
            .on(
                {
                    let calls = Rc::clone(&calls);
                    move |_: &UserCreated| *calls.borrow_mut() += 1
                },
                opts(),
            )
            .unwrap();
        Guard::new(disposer).detach();
    }
    ctx.emit(UserCreated { id: 1 });
    assert_eq!(*calls.borrow(), 1);
}

#[test]
fn guard_explicit_dispose() {
    let ctx = Context::new();
    let guard = Guard::new(ctx.provide(Database::default()).unwrap());
    assert!(ctx.try_get::<Database>().is_some());
    guard.dispose();
    assert!(ctx.try_get::<Database>().is_none());
}
