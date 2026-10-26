# Plan de Integración IA — bpmn-ai

**Estado:** Fase 3 completada ✅  
**Repositorio:** `github.com/Raoon-Tech/bpmn-ai`

---

## Arquitectura General

```
BPMN Engine (Go)
  │
  ├── Core (engine loop, router, failsafe)  ← sin cambios
  ├── Element Registry (factory pattern)    ← +ElementTypeAITask
  ├── Elements (eventos, gateways, activities)
  │     └── AI Task (nuevo)                 ← pkg/ai + ai_task.go
  ├── AI Service Layer
  │     ├── AI Gateway (interface)
  │     ├── Provider Pool (multi-provider)
  │     │     ├── OpenAI Provider
  │     │     └── Anthropic Provider
  │     ├── Model Router (perfiles + capacidades)
  │     ├── Tool Registry (function calling)
  │     ├── Fallback Gateway (primary → secondary)
  │     ├── Guardrail Chain (PII, cost, human-in-the-loop)
  │     ├── Response Cache (memory / redis)
  │     └── Schema Validator (JSON Schema)
  ├── Persistencia
  │     └── AI Audit Log (ai_audit_log)
  └── Observabilidad
        ├── EventAICall
        └── Métricas Prometheus (calls, tokens, latency, cost)
```

### Principio rector

La IA no reemplaza el motor BPMN. La IA vive dentro de flujos BPMN deterministas como un **elemento más** que retorna `ActionRoute`/`ActionError`. El core loop (`runLoop`, `handleResult`, `router`) **no se modifica**.

---

## Fase 1: Fundación ✅ COMPLETADA

### Componentes

| Paquete/Archivo | Propósito |
|-----------------|-----------|
| `pkg/ai/gateway.go` | Interface `Gateway`, `Request`/`Response`, `Message` |
| `pkg/ai/provider.go` | `ProviderRegistry` — registro y creación de providers |
| `pkg/ai/openai.go` | Provider OpenAI (usa `sashabaranov/go-openai`) |
| `pkg/ai/fallback.go` | `FallbackGateway` — primary → secondary |
| `pkg/ai/guardrail.go` | `GuardrailChain` + guardrails built-in |
| `internal/element/activities/ai_task.go` | `AITask` — elemento BPMN que ejecuta llamadas AI |
| `config/config.go` | `AIConfig` con defaults + env vars |
| `pkg/store/store.go` | `AIAuditLogEntry`, `LogAICall()` en `Store` |
| `internal/element/store.go` | `LogAICall()` en `ElementStore` |
| `internal/engine/store.go` | `LogAICall()` en `EngineStore` |
| `pkg/store/memory/store.go` | Implementación in-memory de `LogAICall` |
| `pkg/store/sql/store.go` | Implementación PostgreSQL de `LogAICall` (tabla `ai_audit_log`) |
| `internal/observability/events.go` | Evento `ai.call` |
| `cmd/engine/main.go` | Wiring del AI Gateway |
| `pkg/bpmn/model.go` | +`ElementTypeAITask`, +`ScriptTypeAI`, +`TaskTypeAI` |
| `pkg/bpmn/parser.go` | Reconoce `scriptType="ai"` en XML |

### Configuración

```go
// config/config.go
type AIConfig struct {
    Provider      string        // "openai"
    APIKey        string        // env: AI_API_KEY
    BaseURL       string        // env: AI_BASE_URL (para self-hosted)
    DefaultModel  string        // env: AI_DEFAULT_MODEL, default "gpt-4o"
    FallbackModel string        // env: AI_FALLBACK_MODEL, default "gpt-4o-mini"
    MaxTokens     int           // env: AI_MAX_TOKENS, default 4096
    Temperature   float64       // env: AI_TEMPERATURE, default 0.7
    Timeout       time.Duration // env: AI_TIMEOUT, default 60s
}
```

Variables de entorno:

```bash
AI_PROVIDER=openai              # proveedor (solo openai en Fase 1)
AI_API_KEY=sk-...               # API key
AI_BASE_URL=                    # opcional, para self-hosted/compatibles OpenAI
AI_DEFAULT_MODEL=gpt-4o         # modelo por defecto
AI_FALLBACK_MODEL=gpt-4o-mini   # modelo fallback
AI_MAX_TOKENS=4096
AI_TEMPERATURE=0.7
AI_TIMEOUT=60s
AI_OPENROUTER_API_KEY=sk-or-...   # OpenRouter — acceso a 200+ modelos
```

### BPMN XML — Definición de AI Task

```xml
<scriptTask id="ai-classify" name="Classificar Ticket" scriptType="ai"
    scriptBody="Classifica este ticket: {{ticket_description}}"
    model="gpt-4o"
    systemPrompt="Eres un clasificador de tickets. Responde con una sola palabra.">
</scriptTask>
```

Atributos XML:

| Atributo | Tipo | Obligatorio | Descripción |
|----------|------|-------------|-------------|
| `scriptType` | `"ai"` | Sí | Activa el elemento AITask |
| `scriptBody` | string | Sí | Prompt del usuario. Soporta `{{variable}}` |
| `model` | string | No | Override del modelo default |
| `systemPrompt` | string | No | System prompt para el LLM |
| `outputSchema` | string | No | JSON Schema (reservado para Fase 2) |

### Variables de contexto

El AITask escribe las siguientes variables en el proceso:

| Variable | Contenido |
|----------|-----------|
| `{id}_result` | Texto de respuesta del LLM |
| `{id}_model` | Modelo usado |
| `{id}_tokens_in` | Tokens de input |
| `{id}_tokens_out` | Tokens de output |

Ejemplo: si el elemento tiene `id="ai-1"`, las variables serán `ai-1_result`, `ai-1_model`, etc.

### Store — Tabla AI Audit Log

```sql
CREATE TABLE ai_audit_log (
    id           TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL,
    element_id   TEXT NOT NULL,
    model        TEXT NOT NULL,
    input_text   TEXT,
    output_text  TEXT,
    tokens_in    INTEGER DEFAULT 0,
    tokens_out   INTEGER DEFAULT 0,
    duration_ms  INTEGER DEFAULT 0,
    success      BOOLEAN DEFAULT true,
    error_message TEXT,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);
```

---

## ✅ Fase 2: Madurez (COMPLETADA)

| Componente | Archivo | Detalle |
|------------|---------|---------|
| Provider Anthropic | `pkg/ai/anthropic.go` | Gateway oficial para Claude vía `anthropic-sdk-go` |
| Tool calling | `pkg/ai/tools.go` | `ToolRegistry` + registro de funciones Go como herramientas LLM, loop multi-ronda en OpenAI y Anthropic |
| Output validation | `pkg/ai/schema.go` | Validación JSON Schema de respuestas LLM vía `gojsonschema` |
| Guardrails avanzados | `pkg/ai/guardrail.go` | `CostLimiterGuardrail`, `PIIRedactorGuardrail`, `HumanInTheLoopGuardrail` |
| Métricas Prometheus | `pkg/ai/metrics.go` | 4 métricas: `calls_total`, `tokens_total`, `latency_ms`, `cost_usd_total` |
| Cache de respuestas | `pkg/ai/cache.go` | `Cache` interface + `MemoryCache` + `RedisCache` + `CachedGateway` wrapper |

---

## ✅ Fase 3: Model Router (COMPLETADA)

| Componente | Archivo | Detalle |
|------------|---------|---------|
| Model Router | `pkg/ai/router.go` | `ModelRouter` + `ProviderPool` + catálogo de modelos |
| Perfiles | `pkg/ai/router.go` | `complex`, `fast`, `cheap`, `auto` con defaults configurables |
| Multi-provider | `cmd/engine/main.go` | Pool con OpenAI + Anthropic simultáneos |
| Atributo profile | `pkg/bpmn/parser.go` | `profile="complex|fast|cheap|auto"` en XML |

### Modelos Soportados (35 modelos, 14 proveedores)

Usa formato `provider/model-name` (compatible ai-sdk.dev) o alias cortos.

**OpenAI** — 10 modelos
| Modelo | Contexto | Tools | JSON | Costo In/Out (por 1K) |
|--------|---------|-------|------|-----------------------|
| `openai/gpt-5.5-pro` | 1M | ✅ | ✅ | $0.2727 / $1.6364 |
| `openai/gpt-5.5` | 1M | ✅ | ✅ | $0.03 / $0.18 |
| `openai/gpt-5.5-instant` | 400K | ✅ | ✅ | $0.05 / $0.30 |
| `openai/gpt-5.4` | 1M | ✅ | ✅ | $0.022 / $0.14 |
| `openai/gpt-5.4-mini` | 400K | ✅ | ✅ | $0.0068 / $0.04 |
| `openai/gpt-5.4-nano` | 400K | ✅ | ✅ | $0.0018 / $0.011 |
| `openai/gpt-4o` | 128K | ✅ | ✅ | $0.01 / $0.03 |
| `openai/gpt-4o-mini` | 128K | ✅ | ✅ | $0.0015 / $0.006 |
| `openai/o4-mini` | 200K | ✅ | ✅ | $0.011 / $0.044 |
| `openai/o3` | 200K | ✅ | ✅ | $0.05 / $0.20 |

**Anthropic** — 5 modelos
| Modelo | Contexto | Tools | JSON | Costo In/Out (por 1K) |
|--------|---------|-------|------|-----------------------|
| `anthropic/claude-opus-4-8` | 1M | ✅ | ✅ | $0.0429 / $0.2146 |
| `anthropic/claude-opus-4-7` | 1M | ✅ | ✅ | $0.045 / $0.225 |
| `anthropic/claude-sonnet-4-6` | 200K | ✅ | ✅ | $0.03 / $0.15 |
| `anthropic/claude-haiku-4-5` | 200K | ✅ | ✅ | $0.008 / $0.04 |
| `anthropic/claude-fable-5` | 1M | ✅ | ✅ | $0.10 / $0.50 |

**Google** — 3 modelos
| Modelo | Contexto | Tools | JSON | Costo In/Out (por 1K) |
|--------|---------|-------|------|-----------------------|
| `google/gemini-3.5-flash` | 1M | ✅ | ✅ | $0.0015 / $0.009 |
| `google/gemini-3.1-pro` | 1M | ✅ | ✅ | $0.02 / $0.12 |
| `google/gemini-3.1-flash-lite` | 1M | ✅ | ✅ | $0.0025 / $0.015 |

**DeepSeek** — 3 modelos
| Modelo | Contexto | Tools | JSON | Costo In/Out (por 1K) |
|--------|---------|-------|------|-----------------------|
| `deepseek/deepseek-v4-pro` | 1M | ✅ | ✅ | Gratis |
| `deepseek/deepseek-v4-flash` | 1M | ✅ | ✅ | Gratis |
| `deepseek/deepseek-chat` | 1M | ✅ | ✅ | $0.0014 / $0.0028 |

**Otros proveedores (9):** `mistral/mistral-large-latest`, `meta/llama-4`, `xai/grok-4.3`, `cohere/command-a-plus`, `zhipu/glm-5.2`, `moonshot/kimi-k2.6`, `alibaba/qwen3.6-plus`, `nvidia/nemotron-3-super`, `xiaomi/mimo-v2.5-pro`

**Alias cortos** (resolución automática): `gpt-4o` → `openai/gpt-4o`, `gpt-4o-mini` → `openai/gpt-4o-mini`, `gpt-5.5` → `openai/gpt-5.5`, `claude-sonnet-4-6` → `anthropic/claude-sonnet-4-6`, `claude-haiku-4-5` → `anthropic/claude-haiku-4-5`, `gemini-3.5-flash` → `google/gemini-3.5-flash`, `deepseek-v4-flash` → `deepseek/deepseek-v4-flash`, `deepseek-chat` → `deepseek/deepseek-chat`, `grok-4.3` → `xai/grok-4.3`

**OpenRouter** — modelos vía API gateway unificado (200+ modelos, una sola API key). Formato: `openrouter/provider/model`.

| Modelo | Contexto | Tools | JSON | Costo In/Out (por 1K) |
|--------|---------|-------|------|-----------------------|
| `openrouter/openai/gpt-4o` | 128K | ✅ | ✅ | $0.0025 / $0.01 |
| `openrouter/openai/gpt-4o-mini` | 128K | ✅ | ✅ | $0.00015 / $0.0006 |
| `openrouter/anthropic/claude-sonnet-4-6` | 200K | ✅ | ✅ | $0.009 / $0.045 |
| `openrouter/google/gemini-3.5-flash` | 1M | ✅ | ✅ | $0.0003 / $0.0015 |
| `openrouter/deepseek/deepseek-chat` | 1M | ✅ | ✅ | $0.0007 / $0.0014 |
| `openrouter/meta/llama-4` | 131K | ✅ | ✅ | Gratis |
| `openrouter/mistral/mistral-small` | 256K | ✅ | ✅ | $0.001 / $0.003 |
| `openrouter/xai/grok-4.3` | 1M | ✅ | ✅ | $0.003 / $0.006 |

### Perfiles

| Perfil | Modelo Default | Tokens | Prioridad | Uso recomendado |
|--------|---------------|--------|-----------|-----------------|
| `complex` | `gpt-4o` | 8192 | quality | Análisis profundos, generación de documentos, razonamiento multi-paso |
| `fast` | `gpt-4o-mini` | 2048 | speed | Clasificaciones rápidas, respuestas en tiempo real, chatbots |
| `cheap` | `gpt-4o-mini` | 1024 | cost | Tareas批量 de bajo riesgo, extractos simples, pre-procesamiento |
| `auto` | `gpt-4o` | 4096 | quality | Default. El router elige según capacidades requeridas |

### Resolución de Modelo

El `ModelRouter` sigue estas reglas para seleccionar proveedor+modelo:

1. Si el XML especifica `model="gpt-4o-mini"`, usa ese modelo exacto
2. Si no, usa el `profile` del XML (ej: `profile="fast"`)
3. Si no hay profile, usa la variable `ai_profile` del contexto del proceso
4 Si no, usa el perfil default (`auto`)
5. Valida que el modelo exista en el catálogo y que el proveedor esté configurado
6. Si el modelo requiere tools/schema pero no las soporta, busca el mejor modelo alternativo
7. Si el proveedor del modelo no está disponible, busca en otro proveedor

### Multi-Provider

Para activar múltiples proveedores simultáneamente:

```bash
# Proveedor primario (OpenAI)
AI_PROVIDER=openai
AI_API_KEY=sk-openai-key

# Proveedor secundario (Anthropic) — opcional
AI_ANTHROPIC_API_KEY=sk-ant-key
```

Cuando ambos están configurados, el router puede elegir entre OpenAI y Anthropic según el perfil y las capacidades requeridas por cada tarea.

---

## ✅ Fase 4: Adaptativa (COMPLETADA)

| Componente | Archivo | Detalle |
|------------|---------|---------|
| RAG | `pkg/ai/rag.go` | `RAGSystem` con búsqueda semántica e inyección de contexto en prompts |
| Vector Store | `pkg/ai/vectorstore.go` | `VectorStore` interface + `InMemoryVectorStore` con cosine similarity |
| Embeddings | `pkg/ai/embedding.go` | `Embedder` interface + `OpenAIEmbedder` (text-embedding-3-small) |
| Multi-Agent | `pkg/ai/agent.go` | `MultiAgentExecutor` + `SubAgentTask` — ejecución secuencial de sub-agentes |
| Atributo rag | `pkg/bpmn/parser.go` | `rag="default"` en XML para activar RAG en AI Task |
| Atributo agents | `pkg/bpmn/parser.go` | `agents='[...]'` en XML para tareas multi-agente |

### RAG — Cómo funciona

1. El `RAGSystem` usa un `Embedder` para convertir texto a vectores
2. Los documentos se indexan en un `VectorStore` (implementación in-memory incluida)
3. Cuando un AI Task tiene `rag="default"`, el sistema embediza el prompt
4. Busca los K documentos más similares por cosine similarity
5. Los inyecta como contexto adicional en el prompt antes de llamar al LLM

### Multi-Agent — Cómo funciona

1. El XML define un array JSON de sub-agentes con `name`, `prompt`, `model`, `outputKey`
2. Cada agente se ejecuta secuencialmente con su propio prompt y modelo
3. Los resultados de agentes anteriores se inyectan como variables `{{outputKey}}`
4. Los resultados finales se almacenan como `{id}_agent_{outputKey}`

---

## Dependencias

```go
// Fase 1
require (
    github.com/sashabaranov/go-openai v1.41.2
)
```

```go
// Fase 2 (adicionales)
require (
    github.com/anthropics/anthropic-sdk-go  // Anthropic provider
    github.com/rueian/rueidis               // Redis cache
    go.opentelemetry.io/otel                // Traces
)
```
