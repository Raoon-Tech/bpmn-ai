# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- ORM-backed persistence store (`pkg/store/orm`) on GORM supporting PostgreSQL, MySQL and SQLite.
- Configurable database backend through `DB_DRIVER` (`memory`, `postgres`, `mysql`, `sqlite`) and `DB_*` variables.
- Versioned database migrations runner (`pkg/store/migrate`) with `up`, `down`, `status`, `create`, `auto`, `baseline` and `reset`.
- Command line interface: `serve`, `migrate`, `db`, `config`, `doctor`, `version`.
- `SERVER_DISABLE_CSRF` environment variable.
- Documentation: `docs/persistence.md`, `docs/migrations.md`, `docs/cli.md` (and Spanish versions).

### Changed
- `DATABASE_URL` is now optional; the store is selected explicitly with `DB_DRIVER`.
- Startup fails fast when a configured database cannot be reached (no silent fallback to memory).
- `config.DatabaseConfig` expanded with driver, pooling and migration settings.
- README and AGENTS.md updated to describe the ORM store and CLI.

### Removed
- `pkg/store/sql` (pgx store) and `pkg/store/migrations/001_initial.sql`; replaced by the ORM store and `AutoMigrate`.
- `github.com/ory/dockertest/v3` dependency.

### Breaking
- Existing deployments must set `DB_DRIVER` (default `postgres`) and migrate with `bpmn-ai migrate baseline 0001_initial_schema` when reusing an old pgx database.
- `DATABASE_URL` no longer defaults to `postgres://postgres:postgres@localhost:5432/bpmn`; set it explicitly or use the `DB_*` fields.
