# Migraciones

🌐 [English](migrations.md) | **Español**

## Comandos

```bash
bpmn-ai migrate status
bpmn-ai migrate up [--to VERSION] [--dry-run]
bpmn-ai migrate down --steps N [--force] [--dry-run]
bpmn-ai migrate create <nombre> [--dir DIR]
bpmn-ai migrate auto
bpmn-ai migrate baseline [VERSION] [--all]
bpmn-ai migrate reset --force
```

## Flujo recomendado

1. **Desarrollo**: `DB_AUTO_MIGRATE=true` (por defecto) aplica `migrate up` al arrancar.
2. **Producción**: `DB_AUTO_MIGRATE=false` y ejecutar `bpmn-ai migrate up` en el pipeline de despliegue (init container, job de CI/CD).
3. **Cambios de esquema**: `bpmn-ai migrate create add_priority_to_jobs`, editar `Up`/`Down`, recompilar y ejecutar `migrate up`.

## Create → up → down

```bash
bpmn-ai migrate create add_priority_to_jobs
# edita pkg/store/migrations/<timestamp>_add_priority_to_jobs.go
go build ./...
bpmn-ai migrate up --dry-run
bpmn-ai migrate up
bpmn-ai migrate down --steps 1 --force   # rollback
```

## Limitaciones de AutoMigrate

`AutoMigrate` solo **añade** tablas/columnas e índices. No renombra ni borra. Para esos casos, escribe una migración explícita usando `tx.Migrator()`.

## Lock y concurrencia

- PostgreSQL: `pg_advisory_lock`.
- MySQL: `GET_LOCK`.
- SQLite: serializado por el pool de una sola conexión.

Esto evita que dos instancias del motor apliquen migraciones a la vez.

## Baseline

Para bases de datos existentes creadas con el esquema antiguo (pgx), marca las migraciones como aplicadas sin ejecutarlas:

```bash
bpmn-ai migrate baseline 0001_initial_schema
```

## Reset

`bpmn-ai migrate reset --force` revierte todas las migraciones registradas (solo desarrollo).
