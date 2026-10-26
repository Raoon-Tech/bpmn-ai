package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

func resetRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	registry = map[string]Migration{}
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry = map[string]Migration{}
		registryMu.Unlock()
	})
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := orm.Open(orm.Config{Driver: "sqlite", File: filepath.Join(t.TempDir(), "migrate.db"), LogLevel: "silent"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createTable(name string) func(*gorm.DB) error {
	return func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE " + name + " (id TEXT PRIMARY KEY)").Error
	}
}

func dropTable(name string) func(*gorm.DB) error {
	return func(tx *gorm.DB) error {
		return tx.Exec("DROP TABLE " + name).Error
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	resetRegistry(t)
	Register(Migration{Version: "0001", Up: func(*gorm.DB) error { return nil }})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate version")
		}
	}()
	Register(Migration{Version: "0001", Up: func(*gorm.DB) error { return nil }})
}

func TestUpStatusAndDown(t *testing.T) {
	resetRegistry(t)
	Register(Migration{Version: "0001", Up: createTable("widgets"), Down: dropTable("widgets")})
	Register(Migration{Version: "0002", Up: createTable("gadgets"), Down: dropTable("gadgets")})

	ctx := context.Background()
	db := openTestDB(t)

	applied, err := Up(ctx, db, Options{})
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(applied) != 2 || applied[0] != "0001" || applied[1] != "0002" {
		t.Fatalf("unexpected applied versions: %v", applied)
	}
	if !db.Migrator().HasTable("widgets") || !db.Migrator().HasTable("gadgets") {
		t.Fatal("expected tables to exist")
	}

	again, err := Up(ctx, db, Options{})
	if err != nil {
		t.Fatalf("up again: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected no migrations on second run, got %v", again)
	}

	pending, err := Pending(ctx, db, Options{})
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending migrations, got %v", pending)
	}

	statuses, err := ListStatus(ctx, db)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	for _, st := range statuses {
		if !st.Applied || st.AppliedAt == nil {
			t.Fatalf("expected applied status with timestamp, got %#v", st)
		}
	}

	rolledBack, err := Down(ctx, db, 1, Options{})
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0] != "0002" {
		t.Fatalf("unexpected rollback: %v", rolledBack)
	}
	if db.Migrator().HasTable("gadgets") {
		t.Fatal("expected gadgets table to be dropped")
	}

	pending, err = Pending(ctx, db, Options{})
	if err != nil {
		t.Fatalf("pending after down: %v", err)
	}
	if len(pending) != 1 || pending[0] != "0002" {
		t.Fatalf("unexpected pending migrations: %v", pending)
	}
}

func TestUpToVersion(t *testing.T) {
	resetRegistry(t)
	Register(Migration{Version: "0001", Up: createTable("t1"), Down: dropTable("t1")})
	Register(Migration{Version: "0002", Up: createTable("t2"), Down: dropTable("t2")})
	Register(Migration{Version: "0003", Up: createTable("t3"), Down: dropTable("t3")})

	ctx := context.Background()
	db := openTestDB(t)

	applied, err := Up(ctx, db, Options{To: "0002"})
	if err != nil {
		t.Fatalf("up to: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("expected 2 applied, got %v", applied)
	}
	if db.Migrator().HasTable("t3") {
		t.Fatal("t3 must not exist when stopping at 0002")
	}
}

func TestDryRunDoesNotApply(t *testing.T) {
	resetRegistry(t)
	Register(Migration{Version: "0001", Up: createTable("dry"), Down: dropTable("dry")})

	ctx := context.Background()
	db := openTestDB(t)

	applied, err := Up(ctx, db, Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(applied) != 1 || applied[0] != "0001" {
		t.Fatalf("unexpected dry run result: %v", applied)
	}
	if db.Migrator().HasTable("dry") {
		t.Fatal("dry run must not create the table")
	}
	pending, err := Pending(ctx, db, Options{})
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected the migration to remain pending, got %v", pending)
	}
}

func TestUpRollsBackFailedMigration(t *testing.T) {
	resetRegistry(t)
	Register(Migration{
		Version: "0001",
		Up: func(tx *gorm.DB) error {
			if err := tx.Exec("CREATE TABLE broken (id TEXT PRIMARY KEY)").Error; err != nil {
				return err
			}
			return errors.New("boom")
		},
		Down: dropTable("broken"),
	})

	ctx := context.Background()
	db := openTestDB(t)

	if _, err := Up(ctx, db, Options{}); err == nil {
		t.Fatal("expected migration error")
	}
	if db.Migrator().HasTable("broken") {
		t.Fatal("failed migration must be rolled back")
	}
	statuses, err := ListStatus(ctx, db)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Applied {
		t.Fatalf("failed migration must not be recorded: %#v", statuses)
	}
}

func TestBaseline(t *testing.T) {
	resetRegistry(t)
	Register(Migration{Version: "0001", Up: createTable("b1"), Down: dropTable("b1")})
	Register(Migration{Version: "0002", Up: createTable("b2"), Down: dropTable("b2")})

	ctx := context.Background()
	db := openTestDB(t)

	marked, err := Baseline(ctx, db, "0001", false)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if len(marked) != 1 || marked[0] != "0001" {
		t.Fatalf("unexpected baselined versions: %v", marked)
	}
	if db.Migrator().HasTable("b1") {
		t.Fatal("baseline must not run the migration")
	}

	pending, err := Pending(ctx, db, Options{})
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 || pending[0] != "0002" {
		t.Fatalf("unexpected pending: %v", pending)
	}
}

func TestScaffold(t *testing.T) {
	dir := t.TempDir()
	path, err := Scaffold(dir, "Add Priority To Jobs")
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("expected file in %s, got %s", dir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scaffold: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "migrate.Register") {
		t.Fatalf("scaffolded file does not register a migration:\n%s", content)
	}
	if !strings.Contains(content, "_add_priority_to_jobs.go") && !strings.HasSuffix(path, "_add_priority_to_jobs.go") {
		t.Fatalf("unexpected file name: %s", path)
	}

	second, err := Scaffold(dir, "Add Priority To Jobs")
	if err != nil {
		t.Fatalf("second scaffold: %v", err)
	}
	if second == path {
		t.Fatal("expected a unique file name for the second migration")
	}
}
