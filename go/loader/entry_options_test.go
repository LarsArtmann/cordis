package loader

import (
	"errors"
	"strings"
	"sync"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

// TestUpdateRejectsUncomparableIsolateLabel pins the option-time guard: an
// uncomparable isolate label fails Update and Replace with a field-precise
// error before any option is stored or fiber is restarted, leaving the live
// entry untouched. Context.Isolate would only report the same condition at
// start time with a generic message.
func TestUpdateRejectsUncomparableIsolateLabel(t *testing.T) {
	rec := &recorder{}

	tree := NewTree(cordis.New(), registerEcho(t, rec))
	defer tree.Close()

	if _, err := tree.Create(EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "hi"}}, "", -1); err != nil {
		t.Fatal(err)
	}

	tree.Await()

	for _, label := range []any{[]string{"shared"}, map[string]any{"k": "v"}, func() {}} {
		if err := tree.Update("a", EntryOptions{Isolate: map[string]any{"logger": label}}); err == nil {
			t.Fatalf("Update with a %T isolate label must fail", label)
		} else if !strings.Contains(err.Error(), `isolate["logger"]`) || !strings.Contains(err.Error(), "not comparable") {
			t.Fatalf("error = %v, want a field-precise isolate[\"logger\"] message", err)
		}
	}

	if err := tree.Replace(
		"a",
		EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "hi"}, Isolate: map[string]any{"logger": 42}},
	); err != nil {
		t.Fatalf("Replace with a comparable isolate label must succeed: %v", err)
	}

	tree.Await()

	// The rejected updates left the live fiber untouched: only the initial
	// apply is recorded so far, the accepted Replace rebuilt the scope on
	// top of it.
	events := rec.snapshot()
	if len(events) != 3 {
		t.Fatalf("events = %v, want the initial apply plus exactly one accepted reload", events)
	}

	if events[0] != "start:hi" || events[1] != "stop:hi" || events[2] != "start:hi" {
		t.Fatalf("events = %v, want the accepted isolate reload in order", events)
	}

	e, ok := tree.Lookup("a")
	if !ok {
		t.Fatal("entry a missing")
	}

	if s := fiberState(t, e); s != cordis.StateActive {
		t.Fatalf("entry state = %s, want active under the accepted isolate label", s)
	}
}

// TestRecordErrorLastWriterWins pins the entry error sink's
// last-writer-wins semantics: sequential records leave the last failure,
// and two concurrent failing reloads leave exactly one of the two errors
// (whichever write landed last), never nil and never a mix.
func TestRecordErrorLastWriterWins(t *testing.T) {
	type flakyConf struct {
		Fail string `json:"fail"`
	}

	resolver := NewResolver()
	RegisterType(resolver, "flaky", func(_ *cordis.Context, conf flakyConf) error {
		if conf.Fail != "" {
			return errors.New(conf.Fail)
		}

		return nil
	})

	tree := NewTree(cordis.New(), resolver)
	defer tree.Close()

	if _, err := tree.Create(EntryOptions{ID: "a", Name: "flaky"}, "", -1); err != nil {
		t.Fatal(err)
	}

	tree.Await()

	e, ok := tree.Lookup("a")
	if !ok {
		t.Fatal("entry a missing")
	}

	first, second := errors.New("reload failure one"), errors.New("reload failure two")
	e.recordError(first)
	e.recordError(second)

	if got := e.Err(); !errors.Is(got, second) || errors.Is(got, first) {
		t.Fatalf("Err() = %v, want only the last recorded failure", got)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	for _, fail := range []string{"boom1", "boom2"} {
		go func(msg string) {
			defer wg.Done()

			if err := tree.SetConfig("a", flakyConf{Fail: msg}); err != nil {
				t.Errorf("SetConfig(%q): %v", msg, err)
			}
		}(fail)
	}

	wg.Wait()
	tree.Await()

	got := e.Err()
	if got == nil {
		t.Fatal("a failed reload must be recorded on the entry")
	}

	if !strings.Contains(got.Error(), "boom1") && !strings.Contains(got.Error(), "boom2") {
		t.Fatalf("Err() = %v, want one of the two reload failures", got)
	}
}
