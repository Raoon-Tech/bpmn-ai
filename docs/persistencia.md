# Persistencia y ORM

🌐 [English](persistence.md) | **Español**

## Arquitectura

El motor usa la interfaz `store.Store` (`pkg/store/store.go`). La implementación por defecto es el **store ORM** (`pkg/store/orm`), basado en GORM, que soporta **PostgreSQL, MySQL y SQLite**. La implementación en memoria (`pkg/store/memory`) se mantiene para tests.

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

## Tablas gestionadas

| Tabla | Descripción |
|---|---|
| `processes` | Definiciones BPMN (JSON) |
| `instances` | Casos / instancias de proceso |
| `flows` | Registros de ejecución de flujos |
| `threads` | Hilos de ejecución (paralelismo) |
| `jobs` | Cola de trabajos asíncronos |
| `dead_letters` | Trabajos que agotaron reintentos |
| `execution_log` | Auditoría de ejecución |
| `ai_audit_log` | Auditoría de llamadas a IA |
| `ai_providers` | Proveedores de IA configurados (multi-proveedor) |
| `ai_model_profiles` | Perfiles de enrutado de modelos (complex, fast, cheap, auto) |
| `schema_migrations` | Control de versiones de migraciones |

## Variables de entorno

| Variable | Valores | Default | Descripción |
|---|---|---|---|
| `DB_DRIVER` | `memory`, `postgres`, `mysql`, `sqlite` | `postgres` | Motor de base de datos |
| `DB_HOST` | hostname | `localhost` | Host (postgres/mysql) |
| `DB_PORT` | número | `5432` / `3306` | Puerto |
| `DB_USER` | string | `postgres` / `root` | Usuario |
| `DB_PASSWORD` | string | (vacío) | Contraseña |
| `DB_NAME` | string | `bpmn` | Base de datos |
| `DB_SSLMODE` | `disable`, `require`, ... | `disable` | SSL (postgres) |
| `DB_FILE` | ruta | `./data/bpmn.db` | Archivo (sqlite) |
| `DATABASE_URL` | DSN | (vacío) | Override completo; tiene prioridad |
| `DB_AUTO_MIGRATE` | `true`/`false` | `true` | Aplica migraciones al arrancar |
| `DB_ALLOW_DESTRUCTIVE` | `true`/`false` | `false` | Permite `down`/`reset` sin `--force` |
| `DB_POOL_MAX` | número | `25` | Conexiones máximas |
| `DB_POOL_MIN` | número | `5` | Conexiones inactivas |
| `DB_LOG_LEVEL` | `silent`, `error`, `warn`, `info` | `warn` | Log de GORM |

## Pooling y SQLite

- PostgreSQL y MySQL usan el pool configurado (`DB_POOL_MAX`/`DB_POOL_MIN`).
- SQLite usa **un único writer** (`MaxOpenConns=1`) y activa `busy_timeout`, `journal_mode=WAL` y `foreign_keys` por pragma. Es adecuado para desarrollo y despliegues de una sola instancia.

## Fail-fast

Si `DB_DRIVER` es `postgres`, `mysql` o `sqlite` y la base de datos no es accesible, el motor **aborta el arranque** con un error explícito. No hay fallback silencioso a memoria, para evitar perder datos sin darse cuenta.

## JSON y tipos

Los campos `variables`, `payload` y `definition` se serializan como JSON nativo del motor (`jsonb` en PostgreSQL, `json` en MySQL, `JSON` en SQLite) gracias a `gorm.io/datatypes`.

## Migraciones

Ver [Migraciones](migrations.md). El esquema inicial se crea automáticamente con `AutoMigrate` en la migración `0001_initial_schema`.

## Desarrollo local

```bash
# SQLite, sin infraestructura
DB_DRIVER=sqlite DB_FILE=./data/bpmn.db go run ./cmd/engine

# PostgreSQL con Docker
docker compose up -d db
go run ./cmd/engine
```
