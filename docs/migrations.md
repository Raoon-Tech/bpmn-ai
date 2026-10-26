# Migrations

🌐 **English** | [Español](migraciones.md)

## Commands

```bash
bpmn-ai migrate status
bpmn-ai migrate up [--to VERSION] [--dry-run]
bpmn-ai migrate down --steps N [--force] [--dry-run]
bpmn-ai migrate create <name> [--dir DIR]
bpmn-ai migrate auto
bpmn-ai migrate baseline [VERSION] [--all]
bpmn-ai migrate reset --force
```

## Recommended workflow

1. **Development**: `DB_AUTO_MIGRATE=true` (default) applies `migrate up` on boot.
2. **Production**: `DB_AUTO_MIGRATE=false` and run `bpmn-ai migrate up` in the deployment pipeline (init container, CI/CD job).
3. **Schema changes**: `bpmn-ai migrate create add_priority_to_jobs`, edit `Up`/`Down`, rebuild and run `migrate up`.

## Create → up → down

```bash
bpmn-ai migrate create add_priority_to_jobs
# edit pkg/store/migrations/<timestamp>_add_priority_to_jobs.go
go build ./...
bpmn-ai migrate up --dry-run
bpmn-ai migrate up
bpmn-ai migrate down --steps 1 --force   # rollback
```

## AutoMigrate limitations

`AutoMigrate` only **adds** tables/columns and indexes. It does not rename or drop. For those cases, write an explicit migration using `tx.Migrator()`.

## Locking and concurrency

- PostgreSQL: `pg_advisory_lock`.
- MySQL: `GET_LOCK`.
- SQLite: serialized by the single-connection pool.

This prevents two engine instances from applying migrations at the same time.

## Baseline

For existing databases created with the old pgx schema, mark migrations as applied without running them:

```bash
bpmn-ai migrate baseline 0001_initial_schema
```

## Reset

`bpmn-ai migrate reset --force` rolls back every registered migration (development only).
