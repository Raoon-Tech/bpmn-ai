# CLI Reference

🌐 **English** | [Español](cli-es.md)

## General usage

```bash
bpmn-ai [command] [flags]
```

Without a command, the server starts (`serve`).

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

### doctor

```bash
bpmn-ai doctor
```

Checks configuration, database, pending migrations, audit log, AI and HTTP port.

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

## Exit codes

- `0`: success
- `1`: runtime error
- `2`: usage error
