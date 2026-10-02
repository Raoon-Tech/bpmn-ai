# Getting Started

---

## Prerequisites

- Go 1.26+
- PostgreSQL (optional — defaults to in-memory store for development)
- An AI API key (OpenAI, Anthropic, or OpenRouter)

---

## Installation

```bash
git clone https://github.com/Raoon-Tech/bpmn-ai.git
cd bpmn-ai
go mod download
go build ./cmd/engine/
```

---

## Configuration

Create a `.env` file in the project root:

```bash
# AI Provider (choose one)
AI_OPENROUTER_API_KEY=sk-or-v1-your-key-here
# or
AI_API_KEY=sk-your-openai-key
AI_PROVIDER=openai

# Additional providers
AI_ANTHROPIC_API_KEY=sk-ant-your-key
AI_GROQ_API_KEY=gsk-your-key

# AI defaults
AI_DEFAULT_MODEL=gpt-4o
AI_DEFAULT_PROFILE=auto
AI_MAX_TOKENS=4096

# Database (optional, defaults to in-memory)
DATABASE_URL=postgres://postgres:postgres@localhost:5432/bpmn?sslmode=disable
```

---

## Running the Engine

```bash
# Development
go run ./cmd/engine/

# Production
go build -o bpmn-engine ./cmd/engine/
./bpmn-engine

# Docker
docker build -t bpmn-engine .
docker run -p 8080:8080 --env-file .env bpmn-engine
```

---

## Your First Process

### 1. Create a BPMN Diagram

Create `hello-ai.bpmn`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
  xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
  targetNamespace="http://bpmn.io/schema/bpmn">
  <process id="hello-ai" name="Hello AI Process" isExecutable="true">
    <startEvent id="start-1" />
    <sequenceFlow id="flow-1" sourceRef="start-1" targetRef="ai-task-1" />
    <scriptTask id="ai-task-1" name="Say Hello" scriptType="ai">
      <script>Generate a friendly welcome message for a new customer.</script>
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

### 2. Deploy the Process

The API accepts the BPMN XML as a JSON string field:

```bash
# Read the XML file and POST as JSON
BPMN_XML=$(cat hello-ai.bpmn)
curl -X POST http://localhost:8080/api/v1/processes \
  -H "Content-Type: application/json" \
  -d "$(python3 -c "
import json, sys
with open('hello-ai.bpmn') as f:
    xml = f.read()
print(json.dumps({'name': 'Hello AI Process', 'bpmn_xml': xml}))
")"
```

Or use a simpler inline approach for testing:

```bash
curl -X POST http://localhost:8080/api/v1/processes \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Hello AI Process",
    "bpmn_xml": "<?xml version=\"1.0\" encoding=\"UTF-8\"?><definitions xmlns=\"http://www.omg.org/spec/BPMN/20100524/MODEL\" xmlns:bpmn=\"http://www.omg.org/spec/BPMN/20100524/MODEL\" targetNamespace=\"http://bpmn.io/schema/bpmn\"><process id=\"hello-ai\" name=\"Hello AI Process\" isExecutable=\"true\"><startEvent id=\"start-1\"/><scriptTask id=\"ai-task-1\" name=\"Say Hello\" scriptType=\"ai\"><script>Generate a friendly welcome message for a new customer.</script></scriptTask><endEvent id=\"end-1\"/><sequenceFlow id=\"flow-1\" sourceRef=\"start-1\" targetRef=\"ai-task-1\"/><sequenceFlow id=\"flow-2\" sourceRef=\"ai-task-1\" targetRef=\"end-1\"/></process></definitions>"
  }'
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Hello AI Process",
  "created_at": "2026-06-28T12:00:00Z"
}
```

### 3. Start a Case

```bash
curl -X POST http://localhost:8080/api/v1/processes/550e8400-e29b-41d4-a716-446655440000/start
```

**Response:**
```json
{
  "id": "660e8400-...",
  "process_id": "550e8400-...",
  "status": "IN_PROGRESS",
  "created_at": "2026-06-28T12:00:00Z"
}
```

### 4. Check the Result

```bash
curl http://localhost:8080/api/v1/cases/660e8400-...
```

**Response:**
```json
{
  "id": "660e8400-...",
  "status": "COMPLETED",
  "variables": {
    "ai-task-1_result": "Welcome to our platform! We're excited to have you on board...",
    "ai-task-1_model": "gpt-4o-mini",
    "ai-task-1_tokens_in": 25,
    "ai-task-1_tokens_out": 45,
    "ai-task-1_cost": 0.000105,
    "ai_total_cost": 0.000105
  }
}
```

---

## Testing

```bash
make test              # All tests
make test-unit         # Unit tests only
make test-coverage     # With coverage
go test -count=1 ./... # Without race detector (Windows)
```
