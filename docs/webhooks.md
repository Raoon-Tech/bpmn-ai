# Webhooks salientes

El motor puede notificar a un host externo cuando ocurren eventos de dominio.

## Configuración

```bash
WEBHOOK_URL=https://host.example.com/bpmn/events
WEBHOOK_SECRET=shared-secret
WEBHOOK_EVENTS=*            # o lista separada por comas
```

## Payload

```json
{
  "id": "task.completed-1730000000000000000",
  "type": "task.completed",
  "timestamp": "2026-10-26T12:00:00Z",
  "payload": { "instance_id": "...", "task_id": "..." }
}
```

## Seguridad

- Firma HMAC-SHA256 en el header `X-BPMN-Signature: sha256=<hex>`.
- Header `X-BPMN-Event-Id` para idempotencia.
- Header `X-BPMN-Event-Type` con el tipo de evento.

## Reintentos

1s, 2s, 4s, 8s, 16s y luego se descarta (se registra en el log). El host debe responder `2xx` para considerarse entregado.

## Eventos disponibles

`process.started`, `process.completed`, `process.terminated`, `process.error`, `element.executed`, `element.error`, `task.claimed`, `task.completed`, `job.queued`, `job.completed`, `job.failed`, `job.dead`, `ai.call`.
