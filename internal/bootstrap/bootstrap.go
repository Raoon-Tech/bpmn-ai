// Package bootstrap wires the persistence backend shared by the server and the
// administrative CLI commands.
package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/config"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

// Backend is an opened persistence backend.
type Backend struct {
	Store  store.Store
	DB     *gorm.DB // nil for the in-memory backend
	Driver string
	Close  func() error
}

// Open builds the configured backend. It fails fast: an explicitly configured
// database that cannot be reached is an error, never a silent fallback.
func Open(ctx context.Context, cfg config.DatabaseConfig) (*Backend, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" || driver == config.DBDriverMemory {
		return &Backend{
			Store:  memory.NewStore(),
			Driver: config.DBDriverMemory,
			Close:  func() error { return nil },
		}, nil
	}

	db, err := orm.Open(ORMConfig(cfg))
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access sql pool: %w", err)
	}
	return &Backend{
		Store:  orm.New(db),
		DB:     db,
		Driver: driver,
		Close:  sqlDB.Close,
	}, nil
}

// ORMConfig converts the application database configuration into an ORM one.
func ORMConfig(cfg config.DatabaseConfig) orm.Config {
	return orm.Config{
		Driver:       cfg.Driver,
		URL:          cfg.URL,
		Host:         cfg.Host,
		Port:         cfg.Port,
		User:         cfg.User,
		Password:     cfg.Password,
		Name:         cfg.Name,
		SSLMode:      cfg.SSLMode,
		File:         cfg.File,
		MaxOpenConns: cfg.MaxConns,
		MaxIdleConns: cfg.MinConns,
		MaxIdleTime:  cfg.MaxIdleTime,
		LogLevel:     cfg.LogLevel,
	}
}

// Describe returns a log-safe description of the database target.
func Describe(cfg config.DatabaseConfig) string {
	switch strings.ToLower(cfg.Driver) {
	case "", config.DBDriverMemory:
		return "memory"
	case config.DBDriverSQLite:
		if cfg.URL != "" {
			return "sqlite:" + cfg.URL
		}
		return "sqlite:" + cfg.File
	}

	if cfg.URL != "" {
		if u, err := url.Parse(cfg.URL); err == nil && u.Host != "" {
			name := strings.TrimPrefix(u.Path, "/")
			return fmt.Sprintf("%s://%s/%s", u.Scheme, u.Host, name)
		}
		return cfg.Driver
	}
	return fmt.Sprintf("%s://%s:%d/%s", cfg.Driver, cfg.Host, cfg.Port, cfg.Name)
}
