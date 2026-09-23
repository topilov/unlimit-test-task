package migrations

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var files embed.FS

func Apply(url string) error {
	source, err := iofs.New(files, "migrations")
	if err != nil {
		return err
	}
	url = strings.Replace(url, "postgres://", "pgx5://", 1)
	url = strings.Replace(url, "postgresql://", "pgx5://", 1)
	m, err := migrate.NewWithSourceInstance("iofs", source, url)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	defer m.Close()
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
