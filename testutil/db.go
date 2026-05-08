// Package testutil provides shared test helpers (currently DB bootstrap).
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	_ "github.com/go-sql-driver/mysql"

	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"

	"gorm.io/gorm"

	"github.com/gs97ahn/claude-ops/internal/repository"
)

// NewTestDB starts an isolated MySQL 8.0 testcontainer, applies all
// migrations, and returns a GORM handle. The container is terminated via
// t.Cleanup. Tests that need a fresh DB should call this from each test
// (it intentionally does not cache state).
//
// Requires Docker on the host. Skips with t.Skip if Docker is unavailable
// — callers can rely on standard go test exit codes.
func NewTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	ctx := context.Background()
	container, err := mysqlcontainer.Run(ctx, "mysql:8.0",
		mysqlcontainer.WithDatabase("claude_ops_test"),
		mysqlcontainer.WithUsername("test"),
		mysqlcontainer.WithPassword("testpass"),
	)
	if err != nil {
		t.Skipf("mysql testcontainer unavailable (docker not running?): %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	dsn, err := container.ConnectionString(ctx,
		"parseTime=true",
		"multiStatements=true",
		"charset=utf8mb4",
		"loc=UTC",
	)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := repository.NewDB(repository.DBConfig{
		DSN:          dsn,
		MaxOpenConns: 10,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr != nil {
			return
		}
		_ = sqlDB.Close()
	})

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	driver, err := migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
	if err != nil {
		t.Fatalf("migrate driver: %v", err)
	}
	migrationsDir := findMigrationsDir(t)
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "mysql", driver)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	return db
}

// findMigrationsDir walks up from the current working directory until a
// `migrations/` sibling is found, so tests work from any package depth.
func findMigrationsDir(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find migrations directory")
		}
		dir = parent
	}
}
