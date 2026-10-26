# BPMN-AI Engine

🌐 **English** | [Español](README.es.md)

> Standalone BPMN 2.0 execution engine written in Go with **native AI/LLM integration**. High-performance, production-ready.

📖 **[Read the full documentation →](wiki/Home.md)** — Architecture, BPMN elements, AI integration, API reference.

## Quick Start

```bash
# Clone & run
git clone https://github.com/Raoon-Tech/bpmn-ai.git && cd bpmn-ai
go run ./cmd/engine
```

```bash
# Verify it's running
curl http://localhost:8080/health
```

```bash
# Deploy a process (JSON with BPMN XML string)
curl -X POST http://localhost:8080/api/v1/processes \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Hello AI",
    "bpmn_xml": "<?xml version=\"1.0\"?><definitions xmlns=\"http://www.omg.org/spec/BPMN/20100524/MODEL\" targetNamespace=\"test\"><process id=\"hello-ai\" name=\"Hello AI\" isExecutable=\"true\"><startEvent id=\"s1\"/><scriptTask id=\"ai-1\" name=\"Greet\" scriptType=\"ai\"><script>Say hello in a friendly way</script></scriptTask><endEvent id=\"e1\"/><sequenceFlow id=\"f1\" sourceRef=\"s1\" targetRef=\"ai-1\"/><sequenceFlow id=\"f2\" sourceRef=\"ai-1\" targetRef=\"e1\"/></process></definitions>"
  }'

# Start a case
curl -X POST http://localhost:8080/api/v1/processes/{processID}/start

# Check results
curl http://localhost:8080/api/v1/cases/{caseID}
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        HTTP API (chi)                        │
│  /health  /metrics  /api/v1/processes  /api/v1/cases        │
├─────────────────────────────────────────────────────────────┤
│                     Engine Core (iterative loop)              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │  Engine  │→ │  Router  │→ │ FailSafe │  │  Registry  │  │
│  │  (loop)  │  │          │  │ Manager  │  │ (Factory)  │  │
│  └──────────┘  └──────────┘  └──────────┘  └────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                     BPMN Elements                            │
│  Events · Gateways · Activities · Flows · Boundary Events   │
├─────────────────────────────────────────────────────────────┤
│                  AI System (pkg/ai/)                          │
│  Model Router · Provider Pool · RAG · Guardrails · Cache     │
├─────────────────────────────────────────────────────────────┤
│                     Observability                             │
│  Prometheus · Event Dispatcher · slog · Audit Trail          │
├─────────────────────────────────────────────────────────────┤
│                     Persistence Layer                         │
│  PostgreSQL/MySQL/SQLite (GORM) · In-Memory (tests) · Job Queue │
└─────────────────────────────────────────────────────────────┘
```

---

## AI Integration (🤖)

The engine includes a complete AI system:

| Feature | Description |
|---|---|
| **AITask Element** | `scriptType="ai"` — LLM-powered BPMN element with model selection, tool calling, RAG |
| **Model Router** | 60+ models across 15 providers, profile-based routing (`auto`, `fast`, `cheap`, `complex`) |
| **Multi-Provider** | OpenAI, Anthropic, OpenRouter (200+ models), Groq — automatic failover |
| **RAG** | Semantic document search with embeddings, auto-injects context into prompts |
| **Guardrails** | PII redaction, cost limiting, token limits, human-in-the-loop confidence checks |
| **Tools** | Register Go functions as LLM-callable tools with multi-round execution |
| **Multi-Agent** | Sequential sub-agent orchestration with result passing between agents |
| **Structured Output** | Native JSON mode (OpenAI `json_schema` + Anthropic structured output) |
| **Streaming** | Token streaming with real-time `{id}_partial` variable updates |
| **Cost Tracking** | Per-call and cumulative `ai_total_cost` across the entire process instance |
| **Prompt Versioning** | Named templates with auto-versioning and SHA256 content hashing |
| **Response Cache** | In-memory or Redis-backed cache with TTL |

### XML Example

```xml
<scriptTask id="ai-task" name="Analyze" scriptType="ai"
    scriptBody="Analyze this: {{input_text}}"
    profile="auto" model="gpt-4o"
    systemPrompt="You are an analyst."
    tools="search,calculate"
    rag="knowledge-base"
    outputSchema='{"type":"object","properties":{"summary":{"type":"string"},"score":{"type":"number"}},"required":["summary"]}'/>
```

---

## Supported BPMN Elements

| Category | Elements |
|---|---|
| **Events** | Start, End, Terminate, Timer (ISO 8601/cron), Message Throw/Catch, Error End/Catch, Signal Throw/Catch |
| **Gateways** | Exclusive (XOR), Parallel (AND), Inclusive (OR), Event-Based |
| **Activities** | UserTask, ScriptTask (govaluate), ServiceTask (async), **🤖 AITask**, Sub-Process, CallActivity |
| **Flows** | SequenceFlow, Conditional Flow, Default Flow |
| **Boundary** | Timer (interrupting/non), Message (interrupting), Error |

---

## API Endpoints

| Method | Endpoint | Description |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/ready` | Readiness check |
| GET | `/metrics` | Prometheus metrics |
| POST | `/api/v1/processes` | Deploy process (JSON with `name` + `bpmn_xml`) |
| GET | `/api/v1/processes` | List processes |
| GET | `/api/v1/processes/{id}` | Get process |
| POST | `/api/v1/processes/{id}/start` | Start case |
| GET | `/api/v1/cases` | List cases |
| GET | `/api/v1/cases/{id}` | Case details |
| GET | `/api/v1/cases/{id}/tasks` | Pending tasks |
| GET | `/api/v1/cases/{id}/history` | Execution history |
| GET | `/api/v1/cases/{id}/diagram` | Process diagram |
| POST | `/api/v1/tasks/{id}/claim` | Claim task |
| POST | `/api/v1/tasks/{id}/complete` | Complete task |
| POST | `/api/v1/messages` | Send message |
| POST | `/api/v1/signals` | Broadcast signal |

---

## Project Structure

```
bpmn-ai/
├── cmd/engine/main.go              # CLI entry point, wires deps
├── internal/
│   ├── engine/                     # Core execution engine
│   │   ├── engine.go               # Iterative loop, handleResult, boundary scheduling
│   │   ├── router.go               # Flow routing + thread management
│   │   ├── failsafe.go             # Timeout + loop detection
│   │   └── registry.go             # Element factory registry
│   ├── element/                    # BPMN elements
│   │   ├── events/                 # Start, End, Timer, Message, Error, Signal, Terminate
│   │   ├── gateways/               # Exclusive, Parallel, Inclusive, EventBased
│   │   ├── activities/             # UserTask, ScriptTask, ServiceTask, AITask, SubProcess, CallActivity
│   │   └── flows/                  # SequenceFlow
│   ├── process/state.go            # State machine, instance, thread lifecycle
│   ├── queue/                      # Job queue: worker pool, retry, dead letter
│   └── observability/              # slog, Prometheus, event dispatcher, audit
├── pkg/
│   ├── ai/                         # AI provider system (15+ files)
│   │   ├── gateway.go              # Gateway interface, Message, Request, Response types
│   │   ├── openai.go               # OpenAI provider (Go OpenAI SDK)
│   │   ├── anthropic.go            # Anthropic Claude provider (official SDK)
│   │   ├── router.go               # Model router (60+ models, profiles, aliases)
│   │   ├── provider.go             # Provider registry (factory pattern)
│   │   ├── fallback.go             # Primary → secondary failover
│   │   ├── cross_provider.go       # Ordered multi-provider failover
│   │   ├── cache.go                # Response cache (memory/Redis)
│   │   ├── guardrail.go            # PII, cost, token, HITL guardrails
│   │   ├── rag.go                  # RAG system (embedding + search + enrichment)
│   │   ├── vectorstore.go          # Vector store interface + in-memory impl
│   │   ├── embedding.go            # Embedder interface + OpenAI embedder
│   │   ├── agent.go                # Multi-agent executor
│   │   ├── prompt.go               # Prompt manager (versioned templates)
│   │   ├── tools.go                # Tool registry for function calling
│   │   ├── schema.go               # JSON Schema validator
│   │   └── metrics.go              # Prometheus AI metrics
│   ├── bpmn/                       # BPMN model types + XML parser
│   └── store/                      # Store interface + PostgreSQL + in-memory
├── api/
│   ├── http/                       # REST API (chi router, handlers, routes)
│   └── middleware/                  # RequestID, recovery, logging, CSRF, rate limiter
├── config/config.go                # Configuration system
├── testdata/                       # BPMN XML test files
├── wiki/                           # GitHub wiki documentation
├── Dockerfile & docker-compose.yml
└── Makefile
```

---

## Running Options

| Option | Command |
|---|---|
| Development | `go run ./cmd/engine` |
| Production | `go build -o bpmn-ai ./cmd/engine && ./bpmn-ai` |
| Docker | `docker build -t bpmn-ai . && docker run -p 8080:8080 bpmn-ai` |
| Docker Compose | `docker-compose up -d` |

---

## Database and migrations

The engine persists through an ORM store (GORM) supporting **PostgreSQL, MySQL and SQLite**.

```bash
# SQLite, no infrastructure
DB_DRIVER=sqlite go run ./cmd/engine

# PostgreSQL with Docker
docker compose up -d db
go run ./cmd/engine
```

Key variables: `DB_DRIVER`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_FILE` (sqlite), `DATABASE_URL` (override), `DB_AUTO_MIGRATE`. Details in [docs/persistence.md](docs/persistence.md).

### Migrations

```bash
bpmn-ai migrate status
bpmn-ai migrate up --dry-run
bpmn-ai migrate up
bpmn-ai migrate create add_priority_to_jobs
```

See [docs/migrations.md](docs/migrations.md).

### CLI

```bash
bpmn-ai doctor          # diagnostics
bpmn-ai config show     # effective configuration
bpmn-ai db ping         # database connection
bpmn-ai db stats        # rows per table
bpmn-ai db seed x.bpmn  # deploy a process
```

See [docs/cli.md](docs/cli.md).

## Authentication, multi-tenant and webhooks

The engine **does not manage users or roles**. Identity is delegated to the host via `AUTH_MODE=none|jwt|gateway` (see [docs/auth.md](docs/auth.md)). Each instance can be associated with a tenant (`tenant_id`). Events can be pushed to an external host via signed webhooks (see [docs/webhooks.md](docs/webhooks.md)).

## gRPC

The engine can also expose a gRPC server (`GRPC_ENABLED=true`, `GRPC_PORT=9090`). See [docs/grpc.md](docs/grpc.md).

## Configuration (`.env`)

```bash
# AI Provider
AI_OPENROUTER_API_KEY=sk-or-...    # Takes precedence (200+ models)
# or
AI_API_KEY=sk-...                   # Direct provider key
AI_PROVIDER=openai                  # openai or anthropic

# Additional providers
AI_ANTHROPIC_API_KEY=sk-ant-...
AI_GROQ_API_KEY=gsk_...

# AI defaults
AI_DEFAULT_MODEL=gpt-4o
AI_DEFAULT_PROFILE=auto
AI_MAX_TOKENS=4096
AI_TIMEOUT=60s

# Cache
AI_CACHE_ENABLED=false
AI_CACHE_TYPE=memory

# Database (optional, defaults to in-memory)
DATABASE_URL=postgres://postgres:postgres@localhost:5432/bpmn
```

---

## State Machine

```
CREATED ──→ IN_PROGRESS ──→ COMPLETED
    │            │
    │            ├──→ WAITING ──→ IN_PROGRESS
    │            │       └──→ TERMINATED
    │            ├──→ SUSPENDED ──→ IN_PROGRESS
    │            └──→ ERROR ──→ IN_PROGRESS
    └──→ ERROR
```

---

## Testing

```bash
make test          # All tests
make test-unit     # Unit tests only
make test-coverage # With coverage
make lint          # golangci-lint
```

---

## Documentation

Full documentation is available in the [`wiki/`](wiki/Home.md) directory (bilingual EN/ES):

| English | Español |
|---|---|
| [Home](wiki/Home.md) | [Inicio](wiki/Home-ES.md) |
| [Architecture](wiki/Architecture.md) | [Arquitectura](wiki/Architecture-ES.md) |
| [BPMN Elements](wiki/BPMN-Elements.md) | [Elementos BPMN](wiki/BPMN-Elements-ES.md) |
| [AI Integration](wiki/AI-Integration.md) | [Integración de IA](wiki/AI-Integration-ES.md) |
| [API Reference](wiki/API-Reference.md) | [Referencia de API](wiki/API-Reference-ES.md) |
| [Getting Started](wiki/Getting-Started.md) | [Primeros Pasos](wiki/Getting-Started-ES.md) |
