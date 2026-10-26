package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const (
	// advisoryLockKey is a stable key derived from "BPMN".
	advisoryLockKey int64 = 0x42504D4E
	mysqlLockName         = "bpmn_schema_migrations"
)

// withLock serializes migrations across engine instances. PostgreSQL uses an
// advisory lock and MySQL a named lock; SQLite is serialized by its own writer
// lock and a single connection pool.
func withLock(ctx context.Context, db *gorm.DB, fn func() error) error {
	switch db.Dialector.Name() {
	case "postgres":
		return withPostgresLock(ctx, db, fn)
	case "mysql":
		return withMySQLLock(ctx, db, fn)
	default:
		return fn()
	}
}

func withPostgresLock(ctx context.Context, db *gorm.DB, fn func() error) error {
	conn, err := pinConnection(ctx, db)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockKey)
	}()
	return fn()
}

func withMySQLLock(ctx context.Context, db *gorm.DB, fn func() error) error {
	conn, err := pinConnection(ctx, db)
	if err != nil {
		return err
	}
	defer conn.Close()

	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 30)", mysqlLockName).Scan(&got); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return errors.New("could not acquire migration lock (another migration is running)")
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlLockName)
	}()
	return fn()
}

func pinConnection(ctx context.Context, db *gorm.DB) (*sql.Conn, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access sql pool: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("pin migration connection: %w", err)
	}
	return conn, nil
}
