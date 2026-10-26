# Motor BPMN-AI

🌐 [English](README.md) | **Español**

> Motor de ejecución BPMN 2.0 independiente escrito en Go con **integración nativa de IA/LLM**. Alto rendimiento, listo para producción.

📖 **[Leer la documentación completa →](wiki/Home-ES.md)** — Arquitectura, elementos BPMN, integración de IA, referencia de API.

## Inicio Rápido

```bash
# Clonar y ejecutar
git clone https://github.com/Raoon-Tech/bpmn-ai.git && cd bpmn-ai
go run ./cmd/engine
```

```bash
# Verificar que funciona
curl http://localhost:8080/health
```

```bash
# Desplegar un proceso (JSON con XML BPMN como string)
curl -X POST http://localhost:8080/api/v1/processes \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Hola IA",
    "bpmn_xml": "<?xml version=\"1.0\"?><definitions xmlns=\"http://www.omg.org/spec/BPMN/20100524/MODEL\" targetNamespace=\"test\"><process id=\"hola-ia\" name=\"Hola IA\" isExecutable=\"true\"><startEvent id=\"s1\"/><scriptTask id=\"ai-1\" name=\"Saludar\" scriptType=\"ai\"><script>Saluda de forma amigable</script></scriptTask><endEvent id=\"e1\"/><sequenceFlow id=\"f1\" sourceRef=\"s1\" targetRef=\"ai-1\"/><sequenceFlow id=\"f2\" sourceRef=\"ai-1\" targetRef=\"e1\"/></process></definitions>"
  }'

# Iniciar un caso
curl -X POST http://localhost:8080/api/v1/processes/{processID}/start

# Verificar resultados
curl http://localhost:8080/api/v1/cases/{caseID}
```

---

## Arquitectura

```
┌─────────────────────────────────────────────────────────────┐
│                        HTTP API (chi)                        │
│  /health  /metrics  /api/v1/processes  /api/v1/cases        │
├─────────────────────────────────────────────────────────────┤
│                   Motor Central (bucle iterativo)             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │  Motor   │→ │Enrutador │→ │ FailSafe │  │  Registro  │  │
│  │  (loop)  │  │          │  │ Manager  │  │ (Factory)  │  │
│  └──────────┘  └──────────┘  └──────────┘  └────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                     Elementos BPMN                           │
│  Eventos · Compuertas · Actividades · Flujos · Bordes       │
├─────────────────────────────────────────────────────────────┤
│                  Sistema IA (pkg/ai/)                         │
│  Router Modelos · Pool Proveedores · RAG · Guardrails · Caché│
├─────────────────────────────────────────────────────────────┤
│                     Observabilidad                            │
│  Prometheus · Dispatcher Eventos · slog · Auditoría          │
├─────────────────────────────────────────────────────────────┤
│                     Capa Persistencia                         │
│  PostgreSQL/MySQL/SQLite (GORM) · En Memoria (tests) · Cola Trabajos │
└─────────────────────────────────────────────────────────────┘
```

---

## Integración de IA (🤖)

| Característica | Descripción |
|---|---|
| **Elemento AITask** | `scriptType="ai"` — elemento BPMN potenciado por LLM con selección de modelo, herramientas, RAG |
| **Router de Modelos** | 60+ modelos en 15 proveedores, ruteo por perfil (`auto`, `fast`, `cheap`, `complex`) |
| **Multi-Proveedor** | OpenAI, Anthropic, OpenRouter (200+ modelos), Groq — failover automático |
| **RAG** | Búsqueda semántica con embeddings, inyección automática de contexto en prompts |
| **Guardrails** | Redacción PII, límite de costos, límite de tokens, human-in-the-loop |
| **Herramientas** | Registra funciones Go como herramientas invocables por LLM con ejecución multi-ronda |
| **Multi-Agente** | Orquestación secuencial de sub-agentes con paso de resultados entre agentes |
| **Salida Estructurada** | Modo JSON nativo (OpenAI `json_schema` + Anthropic structured output) |
| **Streaming** | Streaming de tokens con actualización en tiempo real de variable `{id}_partial` |
| **Costo Acumulado** | `ai_total_cost` en toda la instancia + `{id}_cost` por llamada |
| **Versionado Prompts** | Plantillas nombradas con auto-versionado y hash SHA256 |
| **Caché Respuestas** | Caché en memoria o Redis con TTL |

### Ejemplo XML

```xml
<scriptTask id="ai-task" name="Analizar" scriptType="ai"
    scriptBody="Analiza esto: {{input_text}}"
    profile="auto" model="gpt-4o"
    systemPrompt="Eres un analista."
    tools="buscar,calcular"
    rag="base-conocimiento"
    outputSchema='{"type":"object","properties":{"resumen":{"type":"string"},"puntaje":{"type":"number"}},"required":["resumen"]}'/>
```

---

## Elementos BPMN Soportados

| Categoría | Elementos |
|---|---|
| **Eventos** | Start, End, Terminate, Timer (ISO 8601/cron), Message Throw/Catch, Error End/Catch, Signal Throw/Catch |
| **Compuertas** | Exclusive (XOR), Parallel (AND), Inclusive (OR), Event-Based |
| **Actividades** | UserTask, ScriptTask (govaluate), ServiceTask (async), **🤖 AITask**, Sub-Process, CallActivity |
| **Flujos** | SequenceFlow, Conditional Flow, Default Flow |
| **Bordes** | Timer (interrumpente/no), Message (interrumpente), Error |

---

## Endpoints API

| Método | Endpoint | Descripción |
|---|---|---|
| GET | `/health` | Verificación de salud |
| GET | `/ready` | Verificación de disponibilidad |
| GET | `/metrics` | Métricas Prometheus |
| POST | `/api/v1/processes` | Desplegar proceso (JSON con `name` + `bpmn_xml`) |
| GET | `/api/v1/processes` | Listar procesos |
| GET | `/api/v1/processes/{id}` | Obtener proceso |
| POST | `/api/v1/processes/{id}/start` | Iniciar caso |
| GET | `/api/v1/cases` | Listar casos |
| GET | `/api/v1/cases/{id}` | Detalles del caso |
| GET | `/api/v1/cases/{id}/tasks` | Tareas pendientes |
| GET | `/api/v1/cases/{id}/history` | Historial |
| GET | `/api/v1/cases/{id}/diagram` | Diagrama |
| POST | `/api/v1/tasks/{id}/claim` | Reclamar tarea |
| POST | `/api/v1/tasks/{id}/complete` | Completar tarea |
| POST | `/api/v1/messages` | Enviar mensaje |
| POST | `/api/v1/signals` | Transmitir señal |

---

## Estructura del Proyecto

```
bpmn-ai/
├── cmd/engine/main.go              # Punto de entrada CLI
├── internal/
│   ├── engine/                     # Motor de ejecución central
│   │   ├── engine.go               # Bucle iterativo, handleResult, bordes
│   │   ├── router.go               # Enrutamiento de flujo + hilos
│   │   ├── failsafe.go             # Timeout + detección de bucles
│   │   └── registry.go             # Registro factory de elementos
│   ├── element/                    # Elementos BPMN
│   │   ├── events/                 # Start, End, Timer, Message, Error, Signal, Terminate
│   │   ├── gateways/               # Exclusive, Parallel, Inclusive, EventBased
│   │   ├── activities/             # UserTask, ScriptTask, AITask, SubProcess, CallActivity
│   │   └── flows/                  # SequenceFlow
│   ├── process/state.go            # Máquina de estados, hilos
│   ├── queue/                      # Cola: worker pool, reintento, DLQ
│   └── observability/              # slog, Prometheus, eventos, auditoría
├── pkg/
│   ├── ai/                         # Sistema de IA (15+ archivos)
│   │   ├── gateway.go              # Interfaz Gateway, tipos Message/Request/Response
│   │   ├── openai.go               # Proveedor OpenAI
│   │   ├── anthropic.go            # Proveedor Anthropic Claude
│   │   ├── router.go               # Router de modelos (60+ modelos, perfiles)
│   │   ├── provider.go             # Registro de proveedores
│   │   ├── fallback.go             # Failover primario→secundario
│   │   ├── cross_provider.go       # Failover multi-proveedor ordenado
│   │   ├── cache.go                # Caché de respuestas
│   │   ├── guardrail.go            # PII, costo, token, HITL
│   │   ├── rag.go                  # Sistema RAG
│   │   ├── vectorstore.go          # Almacén vectorial
│   │   ├── embedding.go            # Embedder
│   │   ├── agent.go                # Ejecutor multi-agente
│   │   ├── prompt.go               # Gestor de prompts versionados
│   │   ├── tools.go                # Registro de herramientas
│   │   ├── schema.go               # Validador JSON Schema
│   │   └── metrics.go              # Métricas Prometheus IA
│   ├── bpmn/                       # Modelo BPMN + parser XML
│   └── store/                      # Interfaz Store + PostgreSQL + memoria
├── api/
│   ├── http/                       # API REST (chi)
│   └── middleware/                  # RequestID, recovery, logging, CSRF
├── config/config.go                # Sistema de configuración
├── testdata/                       # Archivos BPMN de prueba
├── wiki/                           # Documentación wiki GitHub
├── Dockerfile & docker-compose.yml
└── Makefile
```

---

## Opciones de Ejecución

| Opción | Comando |
|---|---|
| Desarrollo | `go run ./cmd/engine` |
| Producción | `go build -o bpmn-ai ./cmd/engine && ./bpmn-ai` |
| Docker | `docker build -t bpmn-ai . && docker run -p 8080:8080 bpmn-ai` |
| Docker Compose | `docker-compose up -d` |

---

## Base de datos y migraciones

El motor persiste con un store ORM (GORM) que soporta **PostgreSQL, MySQL y SQLite**.

```bash
# SQLite, sin infraestructura
DB_DRIVER=sqlite go run ./cmd/engine

# PostgreSQL con Docker
docker compose up -d db
go run ./cmd/engine
```

Variables principales: `DB_DRIVER`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_FILE` (sqlite), `DATABASE_URL` (override), `DB_AUTO_MIGRATE`. Detalle en [docs/persistencia.md](docs/persistencia.md).

### Migraciones

```bash
bpmn-ai migrate status
bpmn-ai migrate up --dry-run
bpmn-ai migrate up
bpmn-ai migrate create add_priority_to_jobs
```

Ver [docs/migraciones.md](docs/migraciones.md).

### CLI

```bash
bpmn-ai doctor          # diagnostico
bpmn-ai config show     # configuracion efectiva
bpmn-ai db ping         # conexion a la BD
bpmn-ai db stats        # filas por tabla
bpmn-ai db seed x.bpmn  # desplegar un proceso
```

Ver [docs/cli-es.md](docs/cli-es.md).

## Autenticación, multi-tenant y webhooks

El motor **no gestiona usuarios ni roles**. La identidad se delega al host mediante `AUTH_MODE=none|jwt|gateway` (ver [docs/auth-es.md](docs/auth-es.md)). Cada instancia puede asociarse a un tenant (`tenant_id`). Los eventos se pueden notificar a un host externo mediante webhooks firmados (ver [docs/webhooks.md](docs/webhooks.md)).

## gRPC

El motor también puede exponer un servidor gRPC (`GRPC_ENABLED=true`, `GRPC_PORT=9090`). Ver [docs/grpc.md](docs/grpc.md).

## Configuración (`.env`)

```bash
# Proveedor IA
AI_OPENROUTER_API_KEY=sk-or-...    # Toma prioridad (200+ modelos)
# o
AI_API_KEY=sk-...                   # Clave directa del proveedor
AI_PROVIDER=openai                  # openai o anthropic

# Proveedores adicionales
AI_ANTHROPIC_API_KEY=sk-ant-...
AI_GROQ_API_KEY=gsk_...

# Valores predeterminados IA
AI_DEFAULT_MODEL=gpt-4o
AI_DEFAULT_PROFILE=auto
AI_MAX_TOKENS=4096
AI_TIMEOUT=60s

# Caché
AI_CACHE_ENABLED=false
AI_CACHE_TYPE=memory

# Base de datos (opcional, por defecto en memoria)
DATABASE_URL=postgres://postgres:postgres@localhost:5432/bpmn
```

---

## Máquina de Estados

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

## Pruebas

```bash
make test          # Todas las pruebas
make test-unit     # Solo unitarias
make test-coverage # Con cobertura
make lint          # golangci-lint
```

---

## Documentación

Documentación completa en el directorio [`wiki/`](wiki/Home-ES.md) (bilingüe EN/ES):

| English | Español |
|---|---|
| [Home](wiki/Home.md) | [Inicio](wiki/Home-ES.md) |
| [Architecture](wiki/Architecture.md) | [Arquitectura](wiki/Architecture-ES.md) |
| [BPMN Elements](wiki/BPMN-Elements.md) | [Elementos BPMN](wiki/BPMN-Elements-ES.md) |
| [AI Integration](wiki/AI-Integration.md) | [Integración de IA](wiki/AI-Integration-ES.md) |
| [API Reference](wiki/API-Reference.md) | [Referencia de API](wiki/API-Reference-ES.md) |
| [Getting Started](wiki/Getting-Started.md) | [Primeros Pasos](wiki/Getting-Started-ES.md) |
