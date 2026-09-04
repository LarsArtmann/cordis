package loader

import (
	"log/slog"

	cordis "github.com/LarsArtmann/cordis/go"
)

// Loader is the root config service. It owns the root tree, provides
// itself as the "loader" service and starts the configured entries.
type Loader struct {
	tree *Tree
}

// New creates a loader on ctx. A nil resolver creates an empty one. The
// loader is provided as the "loader" service; resolve it with
// cordis.GetNamed[*Loader](ctx, "loader").
func New(ctx *cordis.Context, resolver *Resolver) *Loader {
	l := &Loader{tree: NewTree(ctx, resolver)}
	if _, err := ctx.Provide("loader", l); err != nil {
		slog.Error("loader: cannot provide service", "err", err)
	}
	return l
}

// Tree returns the root tree.
func (l *Loader) Tree() *Tree { return l.tree }

// Start replaces the root group config and awaits the resulting fibers.
func (l *Loader) Start(entries []EntryOptions) error {
	if err := l.tree.Root().Update(entries); err != nil {
		return err
	}
	l.tree.Await()
	return nil
}

// Locate returns the id of the entry owning the fiber, if any.
func (l *Loader) Locate(f *cordis.Fiber) (string, bool) {
	return l.tree.Locate(f)
}

// Close disposes every entry of the loader.
func (l *Loader) Close() {
	l.tree.Close()
}
