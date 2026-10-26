package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

func TestInitialSchemaCreatesEveryTable(t *testing.T) {
	db, err := orm.Open(orm.Config{
		Driver:   "sqlite",
		File:     filepath.Join(t.TempDir(), "schema.db"),
		LogLevel: "silent",
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	applied, err := migrate.Up(context.Background(), db, migrate.Options{})
	if err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if len(applied) != 1 || applied[0] != "0001_initial_schema" {
		t.Fatalf("unexpected applied migrations: %v", applied)
	}

	tables := []string{
		"processes", "instances", "flows", "threads",
		"jobs", "dead_letters", "execution_log", "ai_audit_log",
		"schema_migrations",
	}
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func TestInitialSchemaRollback(t *testing.T) {
	db, err := orm.Open(orm.Config{
		Driver:   "sqlite",
		File:     filepath.Join(t.TempDir(), "rollback.db"),
		LogLevel: "silent",
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	ctx := context.Background()
	if _, err := migrate.Up(ctx, db, migrate.Options{}); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	rolledBack, err := migrate.Down(ctx, db, 1, migrate.Options{})
	if err != nil {
		t.Fatalf("rollback migrations: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0] != "0001_initial_schema" {
		t.Fatalf("unexpected rollback: %v", rolledBack)
	}
	for _, table := range []string{"processes", "instances", "ai_audit_log"} {
		if db.Migrator().HasTable(table) {
			t.Errorf("expected table %q to be dropped", table)
		}
	}
}
