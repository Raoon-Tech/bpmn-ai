package orm

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	mysqldrv "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config holds everything needed to open a GORM connection.
type Config struct {
	Driver       string // postgres, mysql or sqlite
	URL          string // optional full DSN; wins over the individual fields
	Host         string
	Port         int
	User         string
	Password     string
	Name         string
	SSLMode      string
	File         string // SQLite database file
	MaxOpenConns int
	MaxIdleConns int
	MaxIdleTime  time.Duration
	LogLevel     string // silent, error, warn, info
}

// NormalizeDriver lowercases and validates the driver name.
func NormalizeDriver(driver string) (string, error) {
	switch d := strings.ToLower(strings.TrimSpace(driver)); d {
	case "postgres", "postgresql":
		return "postgres", nil
	case "mysql", "mariadb":
		return "mysql", nil
	case "sqlite", "sqlite3":
		return "sqlite", nil
	default:
		return "", fmt.Errorf("unsupported database driver %q (expected postgres, mysql or sqlite)", driver)
	}
}

// DSN builds the driver specific connection string.
func (c Config) DSN() (string, error) {
	driver, err := NormalizeDriver(c.Driver)
	if err != nil {
		return "", err
	}
	switch driver {
	case "postgres":
		return postgresDSN(c)
	case "mysql":
		return mysqlDSN(c)
	case "sqlite":
		return sqliteDSN(c)
	}
	return "", fmt.Errorf("unsupported database driver %q", c.Driver)
}

func postgresDSN(c Config) (string, error) {
	if c.URL != "" {
		return c.URL, nil
	}
	if c.Host == "" {
		return "", fmt.Errorf("postgres host must not be empty")
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	if c.Name == "" {
		return "", fmt.Errorf("postgres database name must not be empty")
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(port)),
		Path:   "/" + c.Name,
	}
	if c.User != "" {
		if c.Password != "" {
			u.User = url.UserPassword(c.User, c.Password)
		} else {
			u.User = url.User(c.User)
		}
	}
	q := url.Values{}
	if c.SSLMode != "" {
		q.Set("sslmode", c.SSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func mysqlDSN(c Config) (string, error) {
	if c.URL != "" {
		if !strings.HasPrefix(strings.ToLower(c.URL), "mysql://") {
			return c.URL, nil
		}
		u, err := url.Parse(c.URL)
		if err != nil {
			return "", fmt.Errorf("parse mysql url: %w", err)
		}
		mc := mysqldrv.NewConfig()
		if u.User != nil {
			mc.User = u.User.Username()
			if pass, ok := u.User.Password(); ok {
				mc.Passwd = pass
			}
		}
		mc.Net = "tcp"
		mc.Addr = u.Host
		mc.DBName = strings.TrimPrefix(u.Path, "/")
		mc.ParseTime = true
		mc.Loc = time.Local
		mc.Params = map[string]string{"charset": "utf8mb4"}
		return mc.FormatDSN(), nil
	}

	if c.Host == "" {
		return "", fmt.Errorf("mysql host must not be empty")
	}
	port := c.Port
	if port == 0 {
		port = 3306
	}
	if c.Name == "" {
		return "", fmt.Errorf("mysql database name must not be empty")
	}

	mc := mysqldrv.NewConfig()
	mc.User = c.User
	mc.Passwd = c.Password
	mc.Net = "tcp"
	mc.Addr = net.JoinHostPort(c.Host, strconv.Itoa(port))
	mc.DBName = c.Name
	mc.ParseTime = true
	mc.Loc = time.Local
	mc.Params = map[string]string{"charset": "utf8mb4"}
	return mc.FormatDSN(), nil
}

func sqliteDSN(c Config) (string, error) {
	const pragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"

	dsn := strings.TrimSpace(c.URL)
	if dsn == "" {
		path := strings.TrimSpace(c.File)
		if path == "" {
			path = "./data/bpmn.db"
		}
		dsn = "file:" + filepath.ToSlash(path)
	}
	if strings.HasPrefix(dsn, "sqlite://") {
		dsn = "file:" + strings.TrimPrefix(dsn, "sqlite://")
	}
	if !strings.HasPrefix(dsn, "file:") {
		dsn = "file:" + filepath.ToSlash(dsn)
	}
	if strings.Contains(dsn, "_pragma=") {
		return dsn, nil
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + pragmas, nil
}

// Open connects to the configured database, tunes the pool and verifies the
// connection with a ping.
func Open(cfg Config) (*gorm.DB, error) {
	driver, err := NormalizeDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}
	dsn, err := cfg.DSN()
	if err != nil {
		return nil, err
	}

	var dialector gorm.Dialector
	switch driver {
	case "postgres":
		dialector = postgres.Open(dsn)
	case "mysql":
		dialector = mysql.Open(dsn)
	case "sqlite":
		dialector = sqlite.Open(dsn)
	}

	db, err := gorm.Open(dialector, &gorm.Config{Logger: newLogger(cfg.LogLevel)})
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", driver, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access sql pool: %w", err)
	}

	if driver == "sqlite" {
		// SQLite has a single writer; serializing connections avoids
		// "database is locked" errors with the job queue workers.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		maxOpen := cfg.MaxOpenConns
		if maxOpen <= 0 {
			maxOpen = 25
		}
		maxIdle := cfg.MaxIdleConns
		if maxIdle < 0 {
			maxIdle = 0
		}
		if maxIdle > maxOpen {
			maxIdle = maxOpen
		}
		sqlDB.SetMaxOpenConns(maxOpen)
		sqlDB.SetMaxIdleConns(maxIdle)
		if cfg.MaxIdleTime > 0 {
			sqlDB.SetConnMaxIdleTime(cfg.MaxIdleTime)
		}
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping %s: %w", driver, err)
	}
	return db, nil
}

// AutoMigrate creates or updates every table managed by the store.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(Models()...)
}

// DropAll drops every table managed by the store (used by migration rollbacks).
func DropAll(db *gorm.DB) error {
	models := Models()
	for i := len(models) - 1; i >= 0; i-- {
		if err := db.Migrator().DropTable(models[i]); err != nil {
			return err
		}
	}
	return nil
}

func newLogger(level string) gormlogger.Interface {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "silent":
		return gormlogger.Default.LogMode(gormlogger.Silent)
	case "error":
		return gormlogger.Default.LogMode(gormlogger.Error)
	case "info":
		return gormlogger.Default.LogMode(gormlogger.Info)
	default:
		return gormlogger.Default.LogMode(gormlogger.Warn)
	}
}
