package cordis

import "fmt"

// Cleanup is a function releasing one resource. Cleanups are invoked in
// reverse registration order when the owning effect or fiber is disposed.
// Panics inside a Cleanup are recovered and routed to the framework logger,
// matching the error handling of the TypeScript runtime.
type Cleanup func()

// Disposer removes a registration ahead of time. Disposers are idempotent:
// calling one twice is a no-op.
type Disposer func()

// EffectMeta describes one registered effect for introspection, mirroring
// Fiber.getEffects() upstream.
type EffectMeta struct {
	Label    string
	Children []EffectMeta
}

// disposeBag is an ordered collection of disposables owned by a fiber or by
// one effect inside a fiber. It implements the effect tree: items registered
// while an effect body runs become children of that effect, and disposal is
// always last in, first out.
//
// disposeBag is not internally synchronized; all mutation happens under the
// core mutex while disposal of user cleanups happens without it. The done
// flags are only toggled under the core mutex via disposeItem.dispose, which
// serializes structural changes, while run functions execute lock free.
type disposeBag struct {
	core  *core
	items []*disposeItem
}

type disposeItem struct {
	label    string
	children *disposeBag
	run      Cleanup
	done     bool
}

func newDisposeBag(c *core) *disposeBag {
	return &disposeBag{core: c}
}

// push registers a leaf cleanup under the core lock and returns its handle.
func (b *disposeBag) push(label string, run Cleanup) *disposeItem {
	b.core.mu.Lock()
	defer b.core.mu.Unlock()
	item := &disposeItem{label: label, run: run}
	b.items = append(b.items, item)
	return item
}

// pushEffect registers a nested effect bag and returns its handle.
func (b *disposeBag) pushEffect(label string, children *disposeBag) *disposeItem {
	b.core.mu.Lock()
	defer b.core.mu.Unlock()
	item := &disposeItem{label: label, children: children}
	b.items = append(b.items, item)
	return item
}

// detach removes the item from its bag without executing it and marks it
// done, so a later dispose is a no-op.
func (item *disposeItem) detach(b *disposeBag) {
	b.core.mu.Lock()
	defer b.core.mu.Unlock()
	item.done = true
	b.detachLocked(item)
}

// detachLocked removes item from the bag without running it.
func (b *disposeBag) detachLocked(item *disposeItem) {
	for i, candidate := range b.items {
		if candidate == item {
			b.items = append(b.items[:i], b.items[i+1:]...)
			return
		}
	}
}

// take atomically drains the bag: it detaches all items under the lock and
// returns them in disposal order (last in, first out).
func (b *disposeBag) take() []*disposeItem {
	b.core.mu.Lock()
	items := make([]*disposeItem, 0, len(b.items))
	for _, item := range b.items {
		if !item.done {
			item.done = true
			items = append(items, item)
		}
	}
	b.items = nil
	b.core.mu.Unlock()
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items
}

// dispose marks the item done, detaches it from its bag and executes it.
// It is idempotent and safe for concurrent use.
func (item *disposeItem) dispose(b *disposeBag) {
	b.core.mu.Lock()
	if item.done {
		b.core.mu.Unlock()
		return
	}
	item.done = true
	b.detachLocked(item)
	b.core.mu.Unlock()
	item.execute(b.core)
}

// execute runs the item without detaching it, used when a whole bag drains.
func (item *disposeItem) execute(c *core) {
	if item.children != nil {
		for _, child := range item.children.take() {
			child.execute(c)
		}
	}
	if item.run != nil {
		c.runCleanup(item.run)
	}
}

// runCleanup executes a user cleanup, converting panics into log entries.
// A failing cleanup never aborts the disposal of its siblings. The call is
// wrapped in an API boundary so fibers notified by the cleanup settle before
// the outermost operation returns.
func (c *core) runCleanup(run Cleanup) {
	c.enter()
	defer c.leave()
	defer func() {
		if r := recover(); r != nil {
			c.logError("", fmt.Errorf("cleanup panicked: %v", r))
		}
	}()
	run()
}

// meta returns the introspection view of the bag.
func (b *disposeBag) meta() []EffectMeta {
	b.core.mu.Lock()
	defer b.core.mu.Unlock()
	result := make([]EffectMeta, 0, len(b.items))
	for _, item := range b.items {
		result = append(result, item.metaLocked())
	}
	return result
}

func (item *disposeItem) metaLocked() EffectMeta {
	meta := EffectMeta{Label: item.label}
	if item.children != nil {
		for _, child := range item.children.items {
			meta.Children = append(meta.Children, child.metaLocked())
		}
	}
	if meta.Children == nil {
		meta.Children = []EffectMeta{}
	}
	return meta
}
