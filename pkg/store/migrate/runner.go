package migrate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ErrIrreversible is returned by migrations that cannot be rolled back.
var ErrIrreversible = errors.New("migration is irreversible")

// Migration is a single versioned schema change.
type Migration struct {
	Version string
	Up      func(tx *gorm.DB) error
	Down    func(tx *gorm.DB) error
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Migration{}
)

// Register adds a migration to the global registry. It panics when the version
// is empty or duplicated, which can only happen because of a programming error.
func Register(m Migration) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if m.Version == "" {
		panic("migrate: migration version must not be empty")
	}
	if m.Up == nil {
		panic("migrate: migration " + m.Version + " has no Up function")
	}
	if _, exists := registry[m.Version]; exists {
		panic("migrate: duplicate migration version " + m.Version)
	}
	registry[m.Version] = m
}

// Registered returns every registered migration sorted by version.
func Registered() []Migration {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Migration, 0, len(registry))
	for _, m := range registry {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// Options controls how a migration run behaves.
type Options struct {
	To     string // stop after this version (inclusive)
	DryRun bool   // report what would run without touching the database
}

// Status describes one migration and whether it has been applied.
type Status struct {
	Version    string
	Applied    bool
	AppliedAt  *time.Time
	DurationMs int64
}

type appliedMigration struct {
	Version    string    `gorm:"column:version;type:varchar(191);primaryKey"`
	AppliedAt  time.Time `gorm:"column:applied_at;not null"`
	DurationMs int64     `gorm:"column:duration_ms;not null;default:0"`
}

// TableName returns the migrations bookkeeping table.
func (appliedMigration) TableName() string { return "schema_migrations" }

// EnsureSchema creates the migration bookkeeping table when missing.
func EnsureSchema(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).AutoMigrate(&appliedMigration{})
}

// Up applies every pending migration in order and returns the applied versions.
func Up(ctx context.Context, db *gorm.DB, opts Options) ([]string, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}

	var applied []string
	err := withLock(ctx, db, func() error {
		done, err := appliedVersions(ctx, db)
		if err != nil {
			return err
		}
		for _, m := range Registered() {
			if done[m.Version] {
				continue
			}
			if opts.To != "" && m.Version > opts.To {
				break
			}
			if opts.DryRun {
				applied = append(applied, m.Version)
				continue
			}
			if err := runUp(ctx, db, m); err != nil {
				return err
			}
			applied = append(applied, m.Version)
		}
		return nil
	})
	return applied, err
}

func runUp(ctx context.Context, db *gorm.DB, m Migration) error {
	start := time.Now()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := m.Up(tx); err != nil {
			return fmt.Errorf("migration %s: %w", m.Version, err)
		}
		record := &appliedMigration{
			Version:    m.Version,
			AppliedAt:  time.Now(),
			DurationMs: time.Since(start).Milliseconds(),
		}
		return tx.Create(record).Error
	})
}

// Down rolls back the last n applied migrations and returns their versions.
func Down(ctx context.Context, db *gorm.DB, steps int, opts Options) ([]string, error) {
	if steps < 1 {
		return nil, fmt.Errorf("steps must be at least 1")
	}
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}

	var rolledBack []string
	err := withLock(ctx, db, func() error {
		records, err := lastApplied(ctx, db, steps)
		if err != nil {
			return err
		}
		for _, record := range records {
			m, ok := lookup(record.Version)
			if !ok {
				return fmt.Errorf("migration %s is applied but not present in this binary", record.Version)
			}
			if opts.DryRun {
				rolledBack = append(rolledBack, m.Version)
				continue
			}
			if err := runDown(ctx, db, m); err != nil {
				return err
			}
			rolledBack = append(rolledBack, m.Version)
		}
		return nil
	})
	return rolledBack, err
}

func runDown(ctx context.Context, db *gorm.DB, m Migration) error {
	if m.Down == nil {
		return fmt.Errorf("migration %s: %w", m.Version, ErrIrreversible)
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := m.Down(tx); err != nil {
			return fmt.Errorf("rollback %s: %w", m.Version, err)
		}
		return tx.Where("version = ?", m.Version).Delete(&appliedMigration{}).Error
	})
}

// Reset rolls back every applied migration, newest first.
func Reset(ctx context.Context, db *gorm.DB, opts Options) ([]string, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	records, err := lastApplied(ctx, db, 0)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return Down(ctx, db, len(records), opts)
}

// Pending returns the versions registered but not applied yet.
func Pending(ctx context.Context, db *gorm.DB, opts Options) ([]string, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	done, err := appliedVersions(ctx, db)
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, m := range Registered() {
		if done[m.Version] {
			continue
		}
		if opts.To != "" && m.Version > opts.To {
			break
		}
		pending = append(pending, m.Version)
	}
	return pending, nil
}

// ListStatus merges the registry with the applied table.
func ListStatus(ctx context.Context, db *gorm.DB) ([]Status, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	records, err := lastApplied(ctx, db, 0)
	if err != nil {
		return nil, err
	}
	byVersion := make(map[string]appliedMigration, len(records))
	for _, r := range records {
		byVersion[r.Version] = r
	}

	out := make([]Status, 0, len(registry))
	for _, m := range Registered() {
		st := Status{Version: m.Version}
		if r, ok := byVersion[m.Version]; ok {
			st.Applied = true
			appliedAt := r.AppliedAt
			st.AppliedAt = &appliedAt
			st.DurationMs = r.DurationMs
		}
		out = append(out, st)
	}
	return out, nil
}

// Baseline marks migrations as applied without running them. When all is true
// every registered migration is marked; otherwise only those up to version.
func Baseline(ctx context.Context, db *gorm.DB, version string, all bool) ([]string, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	var marked []string
	err := withLock(ctx, db, func() error {
		done, err := appliedVersions(ctx, db)
		if err != nil {
			return err
		}
		for _, m := range Registered() {
			if done[m.Version] {
				continue
			}
			if !all && version != "" && m.Version > version {
				break
			}
			record := &appliedMigration{Version: m.Version, AppliedAt: time.Now()}
			if err := db.WithContext(ctx).Create(record).Error; err != nil {
				return err
			}
			marked = append(marked, m.Version)
		}
		return nil
	})
	return marked, err
}

func lookup(version string) (Migration, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	m, ok := registry[version]
	return m, ok
}

func appliedVersions(ctx context.Context, db *gorm.DB) (map[string]bool, error) {
	var records []appliedMigration
	if err := db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(records))
	for _, r := range records {
		done[r.Version] = true
	}
	return done, nil
}

// lastApplied returns the most recent applied migrations, newest first. A limit
// of zero means no limit.
func lastApplied(ctx context.Context, db *gorm.DB, limit int) ([]appliedMigration, error) {
	var records []appliedMigration
	q := db.WithContext(ctx).Order("version DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
