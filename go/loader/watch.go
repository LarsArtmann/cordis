package loader

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// EventConfigUpdate fires after a reload applied a new config. Args:
// (entries []EntryOptions). It mirrors the loader/config-update event
// upstream.
const EventConfigUpdate = "loader/config-update"

// Watcher abstracts config file observation. Implement it with any
// filesystem notification mechanism; Watcher decouples the loader from the
// notification technology.
type Watcher interface {
	// Watch calls onChange whenever the watched config may have changed.
	// It blocks until Close is called; onChange calls are serialized.
	Watch(onChange func())
	Close() error
}

// PollWatcher detects config changes by polling the file's modification
// time and size. It is the stdlib-only default Watcher; swap in an
// inotify/fsnotify implementation when one is available.
type PollWatcher struct {
	path     string
	interval time.Duration

	primed   bool
	lastMod  time.Time
	lastSize int64

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// NewPollWatcher watches path at the given interval (500ms when
// non-positive). The baseline is captured here, so changes are measured
// from the moment of construction rather than the first poll; a missing
// file defers the baseline to the first successful stat inside Watch.
func NewPollWatcher(path string, interval time.Duration) *PollWatcher {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	w := &PollWatcher{
		path:     path,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	w.prime()
	return w
}

func (w *PollWatcher) prime() {
	if info, err := os.Stat(w.path); err == nil {
		w.primed = true
		w.lastMod, w.lastSize = info.ModTime(), info.Size()
	}
}

// Watch runs the poll loop, invoking onChange on every detected change. It
// blocks until Close.
func (w *PollWatcher) Watch(onChange func()) {
	defer close(w.done)
	for {
		if info, err := os.Stat(w.path); err == nil {
			if !w.primed {
				w.primed = true
				w.lastMod, w.lastSize = info.ModTime(), info.Size()
			} else if info.ModTime() != w.lastMod || info.Size() != w.lastSize {
				w.lastMod, w.lastSize = info.ModTime(), info.Size()
				onChange()
			}
		}
		select {
		case <-w.stop:
			return
		case <-time.After(w.interval):
		}
	}
}

// Close stops the poll loop and waits for Watch to return.
func (w *PollWatcher) Close() error {
	w.closeOnce.Do(func() { close(w.stop) })
	<-w.done
	return nil
}

// Reload re-reads the config file and diffs the root group against it: new
// entries start, missing entries are disposed and changed entries update in
// place. A file that fails to parse leaves the running config untouched.
// Per-entry failures are applied for the healthy entries and returned
// joined; consult Tree.Errors for the per-entry detail.
func (l *Loader) Reload() error {
	path := l.configPath()
	if path == "" {
		return fmt.Errorf("loader: no config file to reload")
	}
	l.reloadMu.Lock()
	defer l.reloadMu.Unlock()
	entries, err := LoadFile(path)
	if err != nil {
		return err
	}
	if err := l.Start(entries); err != nil {
		return err
	}
	l.tree.ctx.Emit(EventConfigUpdate, entries)
	return nil
}

// Serve reloads the loader whenever the watcher signals a change. The
// watch loop runs on its own goroutine; concurrent reloads coalesce
// through the reload mutex. Close stops the loop and disposes the tree.
func (l *Loader) Serve(w Watcher) {
	l.mu.Lock()
	if l.watcher != nil {
		l.mu.Unlock()
		return
	}
	l.watcher = w
	done := make(chan struct{})
	l.watchDone = done
	l.mu.Unlock()
	go func() {
		defer close(done)
		w.Watch(func() {
			if err := l.Reload(); err != nil {
				slog.Error("loader: reload failed", "err", err)
			}
		})
	}()
}

func (l *Loader) configPath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// stopWatcher closes the watcher, if any, and waits for its loop.
func (l *Loader) stopWatcher() {
	l.mu.Lock()
	w := l.watcher
	l.watcher = nil
	done := l.watchDone
	l.watchDone = nil
	l.mu.Unlock()
	if w != nil {
		_ = w.Close()
	}
	if done != nil {
		<-done
	}
}
