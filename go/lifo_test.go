package cordis_test

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

// TestDisposalLifoPropertyRandomized drives randomized effect trees:
// effects registered in a random nesting shape must always roll back in
// exact reverse registration order (last in, first out), at any depth.
func TestDisposalLifoPropertyRandomized(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			ctx := cordis.New()

			var registered, disposed []string
			var register func(depth int)
			register = func(depth int) {
				label := fmt.Sprintf("e%d", len(registered))
				registered = append(registered, label)
				_, err := ctx.Effect(func(inner *cordis.Context) error {
					if _, err := inner.Cleanup("cleanup:"+label, func() {
						disposed = append(disposed, label)
					}); err != nil {
						return err
					}
					if depth > 0 && rng.Intn(2) == 0 {
						register(depth - 1)
					}
					return nil
				}, label)
				if err != nil {
					t.Fatal(err)
				}
				if depth > 0 && rng.Intn(2) == 1 {
					register(depth - 1)
				}
			}

			total := 5 + rng.Intn(10)
			for len(registered) < total {
				register(1 + rng.Intn(3))
			}

			ctx.Fiber().Dispose()

			want := slices.Clone(registered)
			slices.Reverse(want)
			if !slices.Equal(disposed, want) {
				t.Fatalf("disposal order %v, want reverse registration %v", disposed, want)
			}
		})
	}
}
