// Command greeter is the fork's example smoke app: a typed Greeter
// service, a typed Greeted event, and the synchronous drain contract (the
// listener has already run when Emit returns).
package main

import (
	"fmt"
	"os"

	cordis "github.com/LarsArtmann/cordis/go"
)

// Greeter is provided under its type identity; consumers look it up with
// cordis.MustGet[*Greeter].
type Greeter struct {
	Name string
}

// Greet renders the greeting this example prints.
func (g *Greeter) Greet(who string) string {
	return fmt.Sprintf("Hello, %s, from %s!", who, g.Name)
}

// Greeted is the typed event emitted after a greeting renders.
type Greeted struct {
	Line string
}

func main() {
	ctx := cordis.New()

	if _, err := cordis.Provide(ctx, &Greeter{Name: "cordis"}); err != nil {
		fmt.Fprintln(os.Stderr, "provide:", err)
		os.Exit(1)
	}

	if _, err := cordis.On[Greeted](ctx, func(g Greeted) {
		fmt.Println(g.Line)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "on:", err)
		os.Exit(1)
	}

	greeter := cordis.MustGet[*Greeter](ctx)
	cordis.Emit(ctx, Greeted{Line: greeter.Greet("world")})
}
