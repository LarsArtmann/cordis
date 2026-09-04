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

var DatabasePlugin = cordis.NewPlugin("database", func(ctx *cordis.Context, cfg DatabaseConfig) error {
	db := &Database{DSN: cfg.DSN}
	_, err := ctx.Provide("database", db)
	return err
})

var UserServicePlugin = cordis.NewPlugin("user-service", func(ctx *cordis.Context, _ struct{}) error {
	db := cordis.MustGet[*Database](ctx, "database")
	_, err := ctx.On("user-created", func(args ...any) any {
		fmt.Println(db.Query("SELECT * FROM users"))
		return nil
	})
	return err
}).Inject("database")

func Example() {
	ctx := cordis.New()

	if _, err := cordis.Start(ctx, UserServicePlugin, struct{}{}); err != nil {
		panic(err)
	}
	// The user service stays pending until the database appears.
	if _, err := cordis.Start(ctx, DatabasePlugin, DatabaseConfig{DSN: "postgres://localhost/app"}); err != nil {
		panic(err)
	}

	ctx.Emit("user-created")

	// Output: rows for SELECT * FROM users via postgres://localhost/app
}
