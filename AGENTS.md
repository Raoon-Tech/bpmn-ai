# AGENTS.md

## What this is

Standalone BPMN 2.0 execution engine written in Go. High-performance, production-ready. Module path: `github.com/Raoon-Tech/bpmn-ai`.

## Project structure

| Directory | Purpose |
|---|---|
| `cmd/engine/` | CLI entry point — wires deps, starts HTTP server, graceful shutdown |
| `internal/cli/` | Command line interface — serve, migrate, db, config, doctor, version |
| `internal/bootstrap/` | Shared persistence wiring (ORM backend) |
| `internal/engine/` | Core execution engine — iterative loop, flow router, fail-safe manager, element registry |
| `internal/element/` | BPMN element implementations — events, gateways, activities, flows |
| `internal/process/` | Process & thread management — state machine, instance lifecycle |
| `internal/queue/` | Job queue system — worker pool, retry policy, dead letter queue |
| `internal/observability/` | Logging, metrics, events — slog, Prometheus, event dispatcher |
| `pkg/bpmn/` | BPMN model types & XML parser |
| `pkg/store/` | Persistence interface + memory implementation (testing) |
| `pkg/store/orm/` | ORM-backed store (GORM) — postgres, mysql, sqlite |
| `pkg/store/migrate/` | Versioned migration runner |
| `pkg/store/migrations/` | Versioned migration files (Go) |
| `api/http/` | REST API with chi router — handlers, routes, health checks |
| `api/grpc/` | gRPC server + proto definition |
| `api/middleware/` | HTTP middleware — logging, recovery, request ID, rate limiter, CSRF |
| `config/` | Configuration system |
| `testdata/` | BPMN test files |

## Key architecture

- **Engine core**: `internal/engine/engine.go` — iterative execution loop with goroutine workers (no recursion)
- **Flow router**: `internal/engine/router.go` — determines next elements, handles parallel branches with thread tracking
- **Fail-safe**: `internal/engine/failsafe.go` — timeout + loop detection
- **Element registry**: `internal/engine/registry.go` — factory pattern for BPMN elements
- **HTTP API**: `api/http/server.go` — chi router with middleware chain (logging, recovery, request ID, CORS, CSRF, rate limiter)
- **Job queue**: `internal/queue/worker.go` — worker pool with configurable concurrency, exponential backoff retry, dead letter queue

## Supported BPMN elements

| Category | Elements | Notes |
|---|---|---|
| **Events** | StartEvent, EndEvent, TerminateEvent | Basic events |
| | TimerEvent | ISO 8601 duration, date, cron. Auto-continue via job queue |
| | MessageThrow, MessageCatch | HTTP `POST /api/v1/messages` endpoint, instance+messageRef correlation |
| | ErrorEndEvent | Throws error (endEvent or intermediateThrowEvent + errorEventDefinition) |
|               | ErrorCatchEvent (boundary)  | Catches errors on sub-process scope. Also for error startEvent |
|               | SignalThrowEvent            | Broadcast signal via `signal_ref`. Pass-through to next flow |
|               | SignalCatchEvent            | Waits for broadcast signal. Found by `SendSignal()` across all instances |
| **Gateways** | ExclusiveGateway | govaluate conditions, default flow, GatewayDirection |
| | ParallelGateway | Thread creation, convergence tracking |
| | InclusiveGateway | Multiple condition paths, GatewayDirection |
| | EventBasedGateway | Armed/resolved tracking, first event wins |
| **Activities** | UserTask | Assignee/groups, WAITING state, interrupting boundary events |
| | ScriptTask | govaluate: business_rule, change_field, assign_team, assign_user, add_related |
| | ServiceTask | Async via JobQueue |
| | Sub-Process (embedded) | Inner XML parsed recursively, flattened with prefixed IDs, synthetic entry flow, exit routing |
| | CallActivity | Loads called process from store, flattens with `ca-{id}.` prefix, synthetic entry flow, executes internal, routes back on completion |
| **Flows** | SequenceFlow | Executable element, synthetic `_synth` flows for routing continuity |
| **Boundary Events** | Timer (interrupting) | Scheduled when activity starts, fires → cancels → routes via boundary output |
| | Message (interrupting) | Flow record created when activity starts, found by SendMessage |
| | Error | Attached to sub-process, caught via findErrorCatch |

## State machine

```
CREATED → IN_PROGRESS → COMPLETED
    │         │
    │         ├──→ WAITING → IN_PROGRESS
    │         │      └──→ TERMINATED
    │         ├──→ SUSPENDED → IN_PROGRESS
    │         └──→ ERROR → IN_PROGRESS
    └──→ ERROR
```

## Conventions

- **Go 1.26+** — use modern patterns, no external DI framework (factory functions)
- **ORM store** — `pkg/store/orm` (GORM) implements `pkg/store.Store` for postgres, mysql and sqlite; `memory` remains for tests. No hand-written SQL in application code
- **chi router** — idiomatic HTTP routing, no reflection overhead
- **slog** — structured logging from stdlib, no external logger dependency
- **Prometheus** — official `client_golang` for metrics
- **Factory pattern** — `ElementRegistry.Register()` for element types, never hardcode
- **Iterative execution** — goroutine workers + channels, no recursion
- **Sub-process flattening** — sub-process XML parsed via `parseSubProcessXML()` (avoids recursion through `Parse()` which would cause infinite loop). Elements flattened into parent process with `{parentID}.{childID}` prefix
- **Error resolution** — errors propagate via `ActionThrowError` → engine finds parent sub-process via ID prefix → searches for matching `ErrorCatchEvent` by `AttachedToRef` + error code
- **Boundary scheduling** — when an element returns `ActionForm`/`ActionWait`, `scheduleBoundaryTimers()` checks all elements with `AttachedToRef == currentID` and creates flow records + jobs for timer/message boundary events

## How to verify changes

```bash
# Run all tests
make test

# Unit tests only
make test-unit

# Tests with coverage
make test-coverage

# Build binary
make build

# Lint
make lint

# Tidy dependencies
make tidy
```

After any changes, run `go test -race -count=1 ./internal/... ./pkg/... ./api/... ./config/...` to verify.

> **Note:** The `-race` flag requires `CGO_ENABLED=1` and a C compiler. On Windows without cgo, use `go test -count=1 ./...`.

## Configuration

Engine is configured via `config/` package with YAML + environment variables. Key settings:

- `server.host` / `server.port` — HTTP listener
- `database.url` — PostgreSQL connection string
- `engine.worker_count` — goroutine worker concurrency
- `engine.max_loops` — loop detection limit
- `engine.execution_timeout` — per-execution timeout
- `engine.queue_poll_interval` — job queue polling frequency
- `log.level` / `log.format` — logging configuration
- `server.disable_csrf` — disables CSRF middleware (useful for tests / local dev)

## Gotchas

- `-race` flag requires `CGO_ENABLED=1` and a C compiler (gcc/MinGW on Windows). Use `go test -count=1 ./...` if cgo is unavailable.
- ServiceTask uses `ActionQueue` — routes to async job queue, process continues without waiting for completion.
- UserTask uses `ActionForm` — instance transitions to `WAITING` state until task is completed externally.
- ParallelGateway creates new threads for each branch — thread index formula: `parentThreadIdx * 10 + branchIndex + 1`.
- The in-memory store (`pkg/store/memory/`) is for testing only — production uses the ORM store (`pkg/store/orm/`) selected by `DB_DRIVER`.
- **Migrations**: `migrate up` applies versioned Go migrations in `pkg/store/migrations/`; `DB_AUTO_MIGRATE=true` (default) runs them on boot. `migrate create` scaffolds a file that must be rebuilt into the binary before use.
- **AutoMigrate vs versioned**: AutoMigrate only adds columns/tables; renames or drops need an explicit migration.
- **SQLite**: pure-Go driver (no cgo), single writer (`MaxOpenConns=1`), WAL + busy timeout enabled. Not recommended for multi-instance deployments.
- **Fail-fast**: a configured database that cannot be reached aborts startup — there is no silent fallback to memory.
- **Auth**: the engine does not manage users or roles. Identity is resolved by `AUTH_MODE` (none/jwt/gateway) and injected into the request context. JWT validation uses `AUTH_JWKS_URL`.
- **Multi-tenant**: `tenant_id` column on instances; filtering is context-based via `store.WithTenant`. Empty tenant = global.
- **Webhooks**: enabled with `WEBHOOK_URL`; HMAC-signed, 5 retries with exponential backoff.
- **Sub-Process**: Inner XML parsed via `xml:",innerxml"`, elements flattened with `{subID}.{origID}` prefix. Synthetic entry flow `{id}_sp_entry` routes into the sub-process. Exit routing via `subprocess_exit_flows` on the internal end event's ExtensionData.
- **Error Events**: `ErrorEndEvent` returns `ActionThrowError`. Engine searches for matching `ErrorCatchEvent` via `AttachedToRef` on parent sub-process. `parentSubProcessID()` extracts parent from element ID prefix (e.g., `sp-1.error-end-1` → `sp-1`).
- **Boundary Events**: `scheduleBoundaryTimers()` creates flow records + timer jobs when an activity enters WAITING. Also creates active flow records for message catch boundaries (found by `SendMessage()`). Interrupting boundaries cancel attached activity flows via `cancelAttachedFlows()`. Non-interrupting (`cancelActivity="false"`) fire without cancellation. Error boundaries on sub-processes and activities found by `findErrorCatch()` (searches by `AttachedToRef` + error code match).
- **Call Activity**: Parses `calledElement` attribute. When executed, the engine loads the called process from the store, flattens its elements with `ca-{id}.` prefix, creates a synthetic entry flow, executes the called process, and routes back to the CallActivity's outgoing flows on completion.
- The `CalculateSchedule` function in `internal/element/events/timer.go` is exported for use by the engine's `scheduleBoundaryTimers`.
- `findErrorCatch()` searches by `AttachedToRef` + error code match. Empty error code catches all errors.
