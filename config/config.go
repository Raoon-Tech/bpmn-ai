package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the engine configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Engine   EngineConfig
	AI       AIConfig
	Auth     AuthConfig
	Webhook  WebhookConfig
	Log      LogConfig
	Audit    AuditConfig
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host           string
	Port           int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxBodySize    int64
	AllowedOrigins []string
	TLSCertFile    string
	TLSKeyFile     string
	DisableCSRF    bool
	GrpcEnabled    bool
	GrpcPort       int
}

// Database driver names accepted by DatabaseConfig.Driver.
const (
	DBDriverMemory   = "memory"
	DBDriverPostgres = "postgres"
	DBDriverMySQL    = "mysql"
	DBDriverSQLite   = "sqlite"
)

// DatabaseConfig holds database connection and migration configuration.
//
// The driver is selected with DB_DRIVER. A complete DATABASE_URL can be used
// instead of the individual DB_* fields; when both are present the URL wins and
// the driver is taken from DB_DRIVER (or inferred from the URL scheme).
type DatabaseConfig struct {
	Driver           string
	URL              string
	Host             string
	Port             int
	User             string
	Password         string
	Name             string
	SSLMode          string
	File             string // SQLite database file
	AutoMigrate      bool
	AllowDestructive bool
	MaxConns         int
	MinConns         int
	MaxIdleTime      time.Duration
	LogLevel         string // silent, error, warn, info
}

// EngineConfig holds BPMN engine configuration.
type EngineConfig struct {
	WorkerCount       int
	MaxLoops          int
	ExecutionTimeout  time.Duration
	QueuePollInterval time.Duration
	MaxRetries        int
}

// AIConfig holds AI provider configuration.
type AIConfig struct {
	Provider       string
	APIKey         string
	BaseURL        string
	DefaultModel   string
	FallbackModel  string
	MaxTokens      int
	Temperature    float64
	Timeout        time.Duration
	Cache          AICacheConfig
	DefaultProfile string
	ExtraProviders string // comma-separated: name:key:url
}

// AIProviderConfig holds configuration for a named AI provider.
type AIProviderConfig struct {
	Provider string // "openai", "anthropic"
	Model    string
	APIKey   string
	BaseURL  string
}

// AICacheConfig holds AI response cache configuration.
type AICacheConfig struct {
	Enabled  bool
	TTL      time.Duration
	Type     string // "memory" or "redis"
	RedisURL string
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level  string // debug, info, warn, error
	Format string // json, text
}

// AuditConfig holds audit log configuration.
type AuditConfig struct {
	Enabled bool
	Dir     string
}

// WebhookConfig holds outbound webhook configuration.
type WebhookConfig struct {
	Enabled bool
	URLs    []string
	Secret  string
	Events  []string // "*" means all events
}

// AuthConfig holds authentication/authorization configuration. The engine does
// not manage users or roles; it only resolves identity provided by the host.
type AuthConfig struct {
	Mode             string   // none, jwt, gateway
	JWKSURL          string
	Issuer           string
	Audience         string
	UserClaim        string   // default "sub"
	RolesClaim       string   // default "roles"
	TenantClaim      string   // default "tenant_id"
	TrustedProxies   []string // CIDRs/IPs trusted in gateway mode
	EnforceTaskRoles bool
}

// Validate checks the auth configuration.
func (a AuthConfig) Validate() error {
	switch a.Mode {
	case "", "none", "jwt", "gateway":
	default:
		return fmt.Errorf("unsupported AUTH_MODE %q (expected none, jwt or gateway)", a.Mode)
	}
	if a.Mode == "jwt" {
		if a.JWKSURL == "" {
			return fmt.Errorf("AUTH_JWKS_URL is required when AUTH_MODE=jwt")
		}
	}
	if a.Mode == "gateway" && len(a.TrustedProxies) == 0 {
		return fmt.Errorf("AUTH_TRUSTED_PROXIES is required when AUTH_MODE=gateway")
	}
	if a.UserClaim == "" {
		// normalized later by middleware, but keep config consistent
	}
	return nil
}

// Default returns a configuration with sensible defaults.
func Default() Config {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	dbDriver := strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER")))
	if dbDriver == "" {
		if dbURL != "" {
			dbDriver = inferDriver(dbURL)
		} else {
			dbDriver = DBDriverPostgres
		}
	}

	dbPort := parseEnvInt("DB_PORT", 0)
	if dbPort == 0 {
		dbPort = defaultPort(dbDriver)
	}

	dbUser := envOrDefault("DB_USER", defaultUser(dbDriver))
	dbName := envOrDefault("DB_NAME", "bpmn")
	dbSSLMode := envOrDefault("DB_SSLMODE", "disable")
	dbFile := envOrDefault("DB_FILE", "./data/bpmn.db")

	return Config{
		Server: ServerConfig{
			Host:           envOrDefault("SERVER_HOST", "0.0.0.0"),
			Port:           parseEnvInt("SERVER_PORT", 8080),
			ReadTimeout:    parseEnvDuration("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:   parseEnvDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			MaxBodySize:    10 << 20, // 10MB
			AllowedOrigins: []string{"https://localhost:3000"},
			DisableCSRF:    parseBoolEnv("SERVER_DISABLE_CSRF", false),
			GrpcEnabled:    parseBoolEnv("GRPC_ENABLED", false),
			GrpcPort:       parseEnvInt("GRPC_PORT", 9090),
		},
		Database: DatabaseConfig{
			Driver:           dbDriver,
			URL:              dbURL,
			Host:             envOrDefault("DB_HOST", "localhost"),
			Port:             dbPort,
			User:             dbUser,
			Password:         os.Getenv("DB_PASSWORD"),
			Name:             dbName,
			SSLMode:          dbSSLMode,
			File:             dbFile,
			AutoMigrate:      parseBoolEnv("DB_AUTO_MIGRATE", true),
			AllowDestructive: parseBoolEnv("DB_ALLOW_DESTRUCTIVE", false),
			MaxConns:         parseEnvInt("DB_POOL_MAX", 25),
			MinConns:         parseEnvInt("DB_POOL_MIN", 5),
			MaxIdleTime:      parseEnvDuration("DB_MAX_IDLE_TIME", 30*time.Minute),
			LogLevel:         strings.ToLower(envOrDefault("DB_LOG_LEVEL", "warn")),
		},
		Engine: EngineConfig{
			WorkerCount:       4,
			MaxLoops:          100,
			ExecutionTimeout:  30 * time.Second,
			QueuePollInterval: 5 * time.Second,
			MaxRetries:        3,
		},
		AI: AIConfig{
			Provider:       envOrDefault("AI_PROVIDER", "openai"),
			APIKey:         os.Getenv("AI_API_KEY"),
			BaseURL:        os.Getenv("AI_BASE_URL"),
			DefaultModel:   envOrDefault("AI_DEFAULT_MODEL", "gpt-4o"),
			FallbackModel:  envOrDefault("AI_FALLBACK_MODEL", "gpt-4o-mini"),
			MaxTokens:      parseEnvInt("AI_MAX_TOKENS", 4096),
			Temperature:    parseEnvFloat("AI_TEMPERATURE", 0.7),
			Timeout:        parseEnvDuration("AI_TIMEOUT", 60*time.Second),
			DefaultProfile: envOrDefault("AI_DEFAULT_PROFILE", "auto"),
			ExtraProviders: os.Getenv("AI_EXTRA_PROVIDERS"),
			Cache: AICacheConfig{
				Enabled:  parseBoolEnv("AI_CACHE_ENABLED", false),
				TTL:      parseEnvDuration("AI_CACHE_TTL", 5*time.Minute),
				Type:     envOrDefault("AI_CACHE_TYPE", "memory"),
				RedisURL: os.Getenv("AI_REDIS_URL"),
			},
		},
		Log: LogConfig{
			Level:  envOrDefault("LOG_LEVEL", "info"),
			Format: envOrDefault("LOG_FORMAT", "json"),
		},
		Audit: AuditConfig{
			Enabled: parseBoolEnv("AUDIT_LOG_ENABLED", true),
			Dir:     envOrDefault("AUDIT_LOG_DIR", "./data/audit"),
		},
		Webhook: WebhookConfig{
			Enabled: len(splitCSV(os.Getenv("WEBHOOK_URL"))) > 0,
			URLs:    splitCSV(os.Getenv("WEBHOOK_URL")),
			Secret:  os.Getenv("WEBHOOK_SECRET"),
			Events:  splitCSV(envOrDefault("WEBHOOK_EVENTS", "*")),
		},
		Auth: AuthConfig{
			Mode:             strings.ToLower(envOrDefault("AUTH_MODE", "none")),
			JWKSURL:          os.Getenv("AUTH_JWKS_URL"),
			Issuer:           os.Getenv("AUTH_ISSUER"),
			Audience:         os.Getenv("AUTH_AUDIENCE"),
			UserClaim:        envOrDefault("AUTH_USER_CLAIM", "sub"),
			RolesClaim:       envOrDefault("AUTH_ROLES_CLAIM", "roles"),
			TenantClaim:      envOrDefault("AUTH_TENANT_CLAIM", "tenant_id"),
			TrustedProxies:   splitCSV(os.Getenv("AUTH_TRUSTED_PROXIES")),
			EnforceTaskRoles: parseBoolEnv("AUTH_ENFORCE_TASK_ROLES", false),
		},
	}
}

func parseEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func parseEnvFloat(key string, defaultVal float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

func parseEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

func parseBoolEnv(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

func envOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

// splitCSV splits a comma separated string into trimmed, non-empty items.
func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// inferDriver derives the database driver from a DSN scheme.
func inferDriver(dsn string) string {
	lower := strings.ToLower(dsn)
	switch {
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		return DBDriverPostgres
	case strings.HasPrefix(lower, "mysql://"):
		return DBDriverMySQL
	case strings.HasPrefix(lower, "sqlite://"), strings.HasPrefix(lower, "file:"), strings.HasSuffix(lower, ".db"), strings.HasSuffix(lower, ".sqlite"):
		return DBDriverSQLite
	default:
		return DBDriverPostgres
	}
}

func defaultPort(driver string) int {
	switch driver {
	case DBDriverMySQL:
		return 3306
	case DBDriverSQLite, DBDriverMemory:
		return 0
	default:
		return 5432
	}
}

func defaultUser(driver string) string {
	switch driver {
	case DBDriverMySQL:
		return "root"
	case DBDriverMemory, DBDriverSQLite:
		return ""
	default:
		return "postgres"
	}
}

// Validate checks the configuration for errors.
func (c Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}
	if c.Engine.WorkerCount < 1 {
		return fmt.Errorf("engine worker count must be at least 1")
	}
	if c.Engine.MaxLoops < 1 {
		return fmt.Errorf("engine max loops must be at least 1")
	}
	if c.Engine.ExecutionTimeout < 1*time.Second {
		return fmt.Errorf("engine execution timeout must be at least 1 second")
	}
	if c.Audit.Enabled && c.Audit.Dir == "" {
		return fmt.Errorf("audit log directory must not be empty when audit is enabled")
	}
	if (c.Server.TLSCertFile == "") != (c.Server.TLSKeyFile == "") {
		return fmt.Errorf("both TLSCertFile and TLSKeyFile must be provided for TLS")
	}
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if err := c.Auth.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate checks the database configuration for errors.
func (d DatabaseConfig) Validate() error {
	switch d.Driver {
	case DBDriverMemory:
		return nil
	case DBDriverPostgres, DBDriverMySQL, DBDriverSQLite:
	default:
		return fmt.Errorf("unsupported DB_DRIVER %q (expected memory, postgres, mysql or sqlite)", d.Driver)
	}

	if d.URL == "" {
		if d.Driver == DBDriverSQLite {
			if strings.TrimSpace(d.File) == "" {
				return fmt.Errorf("DB_FILE must be set when DB_DRIVER=sqlite")
			}
		} else {
			if strings.TrimSpace(d.Host) == "" {
				return fmt.Errorf("DB_HOST must not be empty when DB_DRIVER=%s", d.Driver)
			}
			if d.Port < 1 || d.Port > 65535 {
				return fmt.Errorf("DB_PORT must be between 1 and 65535 when DB_DRIVER=%s", d.Driver)
			}
			if strings.TrimSpace(d.Name) == "" {
				return fmt.Errorf("DB_NAME must not be empty when DB_DRIVER=%s", d.Driver)
			}
		}
	}

	if d.MaxConns < 1 {
		return fmt.Errorf("DB_POOL_MAX must be at least 1")
	}
	if d.MinConns < 0 || d.MinConns > d.MaxConns {
		return fmt.Errorf("DB_POOL_MIN must be between 0 and DB_POOL_MAX")
	}
	switch d.LogLevel {
	case "", "silent", "error", "warn", "info":
	default:
		return fmt.Errorf("unsupported DB_LOG_LEVEL %q (expected silent, error, warn or info)", d.LogLevel)
	}
	return nil
}
