# Primeros Pasos

---

## Prerrequisitos

- Go 1.26+
- PostgreSQL (opcional — usa almacenamiento en memoria por defecto)
- Una clave API de IA (OpenAI, Anthropic u OpenRouter)

---

## Instalación

```bash
git clone https://github.com/Raoon-Tech/bpmn-ai.git
cd bpmn-ai
go mod download
go build ./cmd/engine/
```

---

## Configuración

Crea un archivo `.env` en la raíz del proyecto:

```bash
# Proveedor IA (elige uno)
AI_OPENROUTER_API_KEY=sk-or-v1-tu-clave-aqui
# o
AI_API_KEY=sk-tu-clave-openai
AI_PROVIDER=openai

# Proveedores adicionales
AI_ANTHROPIC_API_KEY=sk-ant-tu-clave
AI_GROQ_API_KEY=gsk-tu-clave

# Valores predeterminados IA
AI_DEFAULT_MODEL=gpt-4o
AI_DEFAULT_PROFILE=auto
AI_MAX_TOKENS=4096

# Base de datos (opcional, por defecto en memoria)
DATABASE_URL=postgres://postgres:postgres@localhost:5432/bpmn?sslmode=disable
```

---

## Ejecutar el Motor

```bash
# Desarrollo
go run ./cmd/engine/

# Producción
go build -o bpmn-engine ./cmd/engine/
./bpmn-engine

# Docker
docker build -t bpmn-engine .
docker run -p 8080:8080 --env-file .env bpmn-engine
```

---

## Tu Primer Proceso

### 1. Crear un Diagrama BPMN

Crea `hola-ia.bpmn`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
  xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
  targetNamespace="http://bpmn.io/schema/bpmn">
  <process id="hola-ia" name="Proceso Hola IA" isExecutable="true">
    <startEvent id="start-1" />
    <sequenceFlow id="flow-1" sourceRef="start-1" targetRef="ai-task-1" />
    <scriptTask id="ai-task-1" name="Saludar" scriptType="ai">
      <script>Genera un mensaje de bienvenida amigable para un nuevo cliente.</script>
      <extensionElements>
        <bpmn:model>gpt-4o-mini</bpmn:model>
        <bpmn:profile>fast</bpmn:profile>
      </extensionElements>
    </scriptTask>
    <sequenceFlow id="flow-2" sourceRef="ai-task-1" targetRef="end-1" />
    <endEvent id="end-1" />
  </process>
</definitions>
```

### 2. Desplegar el Proceso

La API acepta el XML BPMN como un campo string JSON:

```bash
curl -X POST http://localhost:8080/api/v1/processes \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Proceso Hola IA",
    "bpmn_xml": "<?xml version=\"1.0\" encoding=\"UTF-8\"?><definitions xmlns=\"http://www.omg.org/spec/BPMN/20100524/MODEL\" xmlns:bpmn=\"http://www.omg.org/spec/BPMN/20100524/MODEL\" targetNamespace=\"http://bpmn.io/schema/bpmn\"><process id=\"hola-ia\" name=\"Proceso Hola IA\" isExecutable=\"true\"><startEvent id=\"start-1\"/><scriptTask id=\"ai-task-1\" name=\"Saludar\" scriptType=\"ai\"><script>Genera un mensaje de bienvenida amigable para un nuevo cliente.</script></scriptTask><endEvent id=\"end-1\"/><sequenceFlow id=\"flow-1\" sourceRef=\"start-1\" targetRef=\"ai-task-1\"/><sequenceFlow id=\"flow-2\" sourceRef=\"ai-task-1\" targetRef=\"end-1\"/></process></definitions>"
  }'
```

**Respuesta:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Proceso Hola IA",
  "created_at": "2026-06-28T12:00:00Z"
}
```

### 3. Iniciar un Caso

```bash
curl -X POST http://localhost:8080/api/v1/processes/550e8400-e29b-41d4-a716-446655440000/start
```

**Respuesta:**
```json
{
  "id": "660e8400-...",
  "process_id": "550e8400-...",
  "status": "IN_PROGRESS",
  "created_at": "2026-06-28T12:00:00Z"
}
```

### 4. Verificar el Resultado

```bash
curl http://localhost:8080/api/v1/cases/660e8400-...
```

**Respuesta:**
```json
{
  "id": "660e8400-...",
  "status": "COMPLETED",
  "variables": {
    "ai-task-1_result": "¡Bienvenido a nuestra plataforma! Estamos emocionados de tenerte a bordo...",
    "ai-task-1_model": "gpt-4o-mini",
    "ai-task-1_tokens_in": 25,
    "ai-task-1_tokens_out": 45,
    "ai-task-1_cost": 0.000105,
    "ai_total_cost": 0.000105
  }
}
```

---

## Pruebas

```bash
make test              # Todas las pruebas
make test-unit         # Solo pruebas unitarias
make test-coverage     # Con cobertura
go test -count=1 ./... # Sin detector de carreras (Windows)
```
