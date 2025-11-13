package tributarymigrate

import (
	"embed"

	"github.com/uptrace/bun/migrate"
)

// MigrationsFS contains the embedded migration files.
//
//go:embed migrations/*.sql
var sqlMigrations embed.FS

var Migrations = migrate.NewMigrations()

func init() {
	if err := Migrations.Discover(sqlMigrations); err != nil {
		panic(err)
	}
}
