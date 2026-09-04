package cordis_test

import (
	"fmt"

	cordis "github.com/LarsArtmann/cordis/go"
)

type DatabaseConfig struct {
	DSN string
}

type Database struct {
	DSN string
}

func (db *Database) Query(q string) string { return "rows for " + q + " via " + db.DSN }

// UserCreated is a typed event: the event name derives from the type, so
// emitters and listeners cannot drift apart on a string.
type UserCreated struct {
	ID int
}

var DatabasePlugin = cordis.NewPlugin("database", func(ctx *cordis.Context, cfg DatabaseConfig) error {
	_, err := cordis.Provide(ctx, &Database{DSN: cfg.DSN})
	return err
})

var UserServicePlugin = cordis.NewPlugin("user-service", func(ctx *cordis.Context, _ struct{}) error {
	db := cordis.MustGet[*Database](ctx)
	_, err := cordis.On(ctx, func(event UserCreated) {
		fmt.Println(db.Query("SELECT * FROM users"))
	})
	return err
}).Inject(cordis.ServiceName[*Database]())

func Example() {
	ctx := cordis.New()

	if _, err := cordis.Start(ctx, UserServicePlugin, struct{}{}); err != nil {
		panic(err)
	}
	// The user service stays pending until the database appears.
	if _, err := cordis.Start(ctx, DatabasePlugin, DatabaseConfig{DSN: "postgres://localhost/app"}); err != nil {
		panic(err)
	}

	cordis.Emit(ctx, UserCreated{ID: 1})

	// Output: rows for SELECT * FROM users via postgres://localhost/app
}
