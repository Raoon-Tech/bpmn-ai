# CLI Reference

🌐 **English** | [Español](cli-es.md)

## General usage

```bash
bpmn-ai [command] [flags]
```

Without a command, the server starts (`serve`). Flags may be placed before or after positional arguments.

## Commands

### serve

```bash
bpmn-ai serve
```

Starts the HTTP engine. Applies pending migrations when `DB_AUTO_MIGRATE=true`.

### version

```bash
bpmn-ai version [--json]
```

Prints version, commit and build date.

### doctor

```bash
bpmn-ai doctor
```

Checks configuration, database connection, pending migrations, audit log directory, AI configuration and HTTP port availability.

### config show

```bash
bpmn-ai config show [--json]
```

Prints the effective configuration with secrets masked.

### migrate

See [Migrations](migrations.md).

### db

```bash
bpmn-ai db ping
bpmn-ai db stats
bpmn-ai db seed <file.bpmn>
```

- `ping`: checks the connection and prints the database engine version.
- `stats`: counts rows per table.
- `seed`: deploys a BPMN file into the database.

### process

- `process list`: lists deployed process definitions (id, name, version, created at).
- `process get <id>`: prints process metadata and element/flow counts.
- `process deploy <file.bpmn>`: parses and stores a BPMN file (equivalent to `db seed`).
- `process delete <id> [--force]`: removes a process definition. Destructive: requires `--force` or `DB_ALLOW_DESTRUCTIVE=true`.

```bash
bpmn-ai process list
bpmn-ai process get 96870a21-8414-4d9d-9de6-67cf735bbc6e
bpmn-ai process deploy examples/approval-process.bpmn
bpmn-ai process delete 96870a21-... --force
```

### case

- `case list [--status STATUS] [--limit N]`: lists instances, optionally filtered by status (`CREATED`, `IN_PROGRESS`, `WAITING`, `COMPLETED`, `ERROR`, `TERMINATED`).
- `case get <id>`: prints status, variables and timestamps.
- `case start <processID> [--title T] [--var k=v]... [--timeout 30s]`: creates a case and runs the engine **synchronously** until it completes or waits; prints the final status and pending tasks.
- `case history <id>`: prints the execution log of the case.
- `case cancel <id>`: marks the case as `TERMINATED`.

```bash
bpmn-ai case start 96870a21-... --var approved=true --title "Order 42"
bpmn-ai case list --status WAITING
bpmn-ai case get 04a1df0f-551d-4e08-9a21-00c359c86752
bpmn-ai case history 04a1df0f-...
bpmn-ai case cancel 04a1df0f-...
```

### task

- `task list <caseID>`: lists the active user tasks of a case (flow id, element, thread, started at).
- `task claim <id> [--user USER]`: claims a task for a user.
- `task complete <id> [--var k=v]... [--timeout 30s]`: completes the task, updates the flow and **resumes the case** synchronously.

```bash
bpmn-ai task list 04a1df0f-...
bpmn-ai task claim dbb944cb-... --user ana
bpmn-ai task complete dbb944cb-... --var approved=true
```

### job

- `job list [--status STATUS] [--limit N]`: lists pending jobs (only `PENDING` is queryable).
- `job retry <id>`: requeues a job (status `PENDING`, retry counter reset, scheduled now).
- `job drain [--limit N] [--timeout 5m]`: processes up to `N` pending jobs **synchronously in the foreground** using the engine handler, until the queue is empty or the timeout expires. Useful for cron, CI or debugging without the background worker.

```bash
bpmn-ai job list
bpmn-ai job retry 1234abcd-...
bpmn-ai job drain --limit 50
```

### message

In BPMN, a **MessageCatch** event (intermediate catch or boundary message event) leaves the instance in `WAITING` until a message with that reference arrives. `message send` delivers the message to one specific instance and resumes it.

```bash
bpmn-ai message send 04a1df0f-... order-confirmed --var order_id=123
```

Useful for integrating external systems, testing message correlation, or completing cases manually without the HTTP API.

### signal

A **SignalThrow** broadcasts a signal to **all** instances that have a waiting **SignalCatch** for that reference (global broadcast, not tied to a single instance). Returns the list of affected instance ids.

```bash
bpmn-ai signal send customer-updated --var tier=premium
```

Useful for global notifications (shared state changes, domain events) and for testing broadcast scenarios.

### ai

Manages the multi-provider AI configuration stored in the `ai_providers` table.

- `ai provider list`: lists configured providers (keys masked).
- `ai provider add <name> --type openai|anthropic --key <key> [--base-url URL] [--model MODEL] [--default] [--key-stdin]`: adds or updates a provider. Use `--key-stdin` to read the key from stdin and keep it out of the shell history.
- `ai provider remove <name>`: removes a provider.
- `ai provider show <name> [--reveal]`: shows a provider (key masked unless `--reveal`).
- `ai provider set-default <name>`: marks a provider as the default.

```bash
bpmn-ai ai provider add openrouter --type openai --key sk-or-... --base-url https://openrouter.ai/api/v1 --default
echo sk-ant-... | bpmn-ai ai provider add anthropic --type anthropic --key-stdin
bpmn-ai ai provider list
bpmn-ai ai provider show openrouter
bpmn-ai ai provider set-default anthropic
bpmn-ai ai provider remove openrouter
```

Keys are stored in plaintext in the database; restrict database access accordingly. Providers loaded from the database are added to the model router at startup (environment variables keep precedence for the primary gateway).

### ai profile

Manages model routing profiles (`complex`, `fast`, `cheap`, `auto`) stored in the `ai_model_profiles` table.

```bash
bpmn-ai ai profile list
bpmn-ai ai profile get <name>
bpmn-ai ai profile set <name> --model <model> [--max-tokens N] [--priority quality|speed|cost]
bpmn-ai ai profile reset [--name <name>]
```

Profiles stored in the database override the built-in defaults at startup. `reset` without `--name` removes all stored profiles (built-in defaults apply).

## Exit codes

- `0`: success
- `1`: runtime error
- `2`: usage error
