# Persistence and ORM

🌐 **English** | [Español](persistencia.md)

## Architecture

The engine uses the `store.Store` interface (`pkg/store/store.go`). The default implementation is the **ORM store** (`pkg/store/orm`), built on GORM, supporting **PostgreSQL, MySQL and SQLite**. The in-memory implementation (`pkg/store/memory`) is kept for tests.

```
config.DatabaseConfig
        │
        ▼
internal/bootstrap.Open
        │
        ├── DB_DRIVER=memory  →  memory.NewStore()
        └── DB_DRIVER=postgres/mysql/sqlite
                │
                ▼
            orm.Open(cfg)  →  *gorm.DB
                │
                ▼
            orm.New(db)  →  store.Store
```

## Managed tables

| Table | Description |
|---|---|
| `processes` | BPMN definitions (JSON) |
| `instances` | Process instances (cases) |
| `flows` | Flow execution records |
| `threads` | Execution threads (parallelism) |
| `jobs` | Asynchronous job queue |
| `dead_letters` | Jobs that exhausted retries |
| `execution_log` | Execution audit log |
| `ai_audit_log` | AI call audit log |
| `ai_providers` | Configured AI providers (multi-provider) |
| `ai_model_profiles` | Model routing profiles (complex, fast, cheap, auto) |
| `schema_migrations` | Migration version control |

## Environment variables

| Variable | Values | Default | Description |
|---|---|---|---|
| `DB_DRIVER` | `memory`, `postgres`, `mysql`, `sqlite` | `postgres` | Database engine |
| `DB_HOST` | hostname | `localhost` | Host (postgres/mysql) |
| `DB_PORT` | number | `5432` / `3306` | Port |
| `DB_USER` | string | `postgres` / `root` | User |
| `DB_PASSWORD` | string | (empty) | Password |
| `DB_NAME` | string | `bpmn` | Database |
| `DB_SSLMODE` | `disable`, `require`, ... | `disable` | SSL (postgres) |
| `DB_FILE` | path | `./data/bpmn.db` | File (sqlite) |
| `DATABASE_URL` | DSN | (empty) | Full override; takes precedence |
| `DB_AUTO_MIGRATE` | `true`/`false` | `true` | Apply migrations on boot |
| `DB_ALLOW_DESTRUCTIVE` | `true`/`false` | `false` | Allow `down`/`reset` without `--force` |
| `DB_POOL_MAX` | number | `25` | Max connections |
| `DB_POOL_MIN` | number | `5` | Idle connections |
| `DB_LOG_LEVEL` | `silent`, `error`, `warn`, `info` | `warn` | GORM log level |

## Pooling and SQLite

- PostgreSQL and MySQL use the configured pool (`DB_POOL_MAX`/`DB_POOL_MIN`).
- SQLite uses a **single writer** (`MaxOpenConns=1`) with `busy_timeout`, `journal_mode=WAL` and `foreign_keys` pragmas. Suitable for development and single-instance deployments.

## Fail-fast

If `DB_DRIVER` is `postgres`, `mysql` or `sqlite` and the database is unreachable, the engine **aborts startup** with an explicit error. There is no silent fallback to memory.

## JSON and types

The `variables`, `payload` and `definition` fields are serialized as the engine's native JSON type (`jsonb` on PostgreSQL, `json` on MySQL, `JSON` on SQLite) via `gorm.io/datatypes`.

## Migrations

See [Migrations](migrations.md). The initial schema is created automatically with `AutoMigrate` in migration `0001_initial_schema`.

## Local development

```bash
# SQLite, no infrastructure
DB_DRIVER=sqlite DB_FILE=./data/bpmn.db go run ./cmd/engine

# PostgreSQL with Docker
docker compose up -d db
go run ./cmd/engine
```
