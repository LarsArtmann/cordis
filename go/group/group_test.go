package group

import (
	"strings"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

func TestGroupCreateRemoveUpdate(t *testing.T) {
	ctx := cordis.New()
	g, err := Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var log []string
	factory := func(name string) Factory {
		return func(c *cordis.Context) error {
			log = append(log, "start "+name)
			_, _ = c.Cleanup("child", func() { log = append(log, "stop "+name) })
			return nil
		}
	}

	if err := g.Create("a", factory("a")); err != nil {
		t.Fatal(err)
	}
	if err := g.Create("b", factory("b")); err != nil {
		t.Fatal(err)
	}
	if err := g.Create("a", factory("dup")); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatal("duplicate Create must fail, got", err)
	}
	if !g.Get("a") || !g.Get("b") {
		t.Fatal("entries must be live after Create")
	}

	if err := g.Update(map[string]Factory{"b": factory("b")}); err != nil {
		t.Fatal(err)
	}
	if g.Get("a") {
		t.Fatal("Update must remove entries missing from the wanted set")
	}
	if !g.Get("b") {
		t.Fatal("Update must keep entries present in the wanted set")
	}
	if got := strings.Join(g.IDs(), ","); got != "b" {
		t.Fatal("IDs broken:", got)
	}

	if err := g.Update(map[string]Factory{"c": factory("c")}); err != nil {
		t.Fatal(err)
	}
	if !g.Get("c") || g.Get("b") {
		t.Fatal("Update diff broken")
	}
	if strings.Join(log, "|") != "start a|start b|stop a|stop b|start c" {
		t.Fatal("lifecycle log broken:", log)
	}
}

func TestGroupRollsBackWithFiber(t *testing.T) {
	ctx := cordis.New()
	var group *Group
	fiber, err := cordis.Start(ctx, cordis.NewPlugin("group-host", func(c *cordis.Context, _ int) error {
		var err error
		group, err = Start(c)
		if err != nil {
			return err
		}
		return group.Create("inner", func(*cordis.Context) error { return nil })
	}), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := fiber.Await(); err != nil {
		t.Fatal(err)
	}
	if !group.Get("inner") {
		t.Fatal("child must be live")
	}
	fiber.Dispose()
	if group.Get("inner") {
		t.Fatal("child must roll back with the group's fiber")
	}
}

func TestGroupFactoryError(t *testing.T) {
	ctx := cordis.New()
	g, err := Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Create("bad", func(*cordis.Context) error {
		return cordis.ErrInactiveEffect
	}); err != nil {
		t.Fatal(err)
	}
	if err := g.ctx.Fiber().Await(); err != nil {
		t.Fatal(err)
	}
	if g.Get("bad") {
		t.Fatal("failed entry must not count as live")
	}
	if g.State("bad") != cordis.StateFailed {
		t.Fatal("failed entry must surface its state, got", g.State("bad"))
	}
}
