# gRPC API

The engine can expose a gRPC server in addition to (or instead of) the REST API.

## Enable

```bash
GRPC_ENABLED=true
GRPC_PORT=9090
```

## Service

The proto definition is in `api/grpc/bpmn.proto`. Services:

- `HealthService.Check`
- `ProcessService`: DeployProcess, GetProcess, ListProcesses, DeleteProcess
- `CaseService`: StartCase, GetCase, ListCases, CancelCase, GetCaseHistory
- `TaskService`: ListTasks, ClaimTask, CompleteTask
- `MessageService`: SendMessage
- `SignalService`: SendSignal

Reflection is enabled, so `grpcurl` and similar tools work.

## Auth

The same `AUTH_MODE` applies. In gRPC, credentials are passed via metadata (`authorization: Bearer <token>` for JWT, or `x-user-id`/`x-user-roles`/`x-tenant-id` for gateway mode).

## Generate clients

Stubs are generated with `protoc`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
protoc --go_out=api/grpc/pb --go_opt=paths=source_relative \
       --go-grpc_out=api/grpc/pb --go-grpc_opt=paths=source_relative \
       api/grpc/bpmn.proto
```

For Python/PHP/JS, use `protoc` with the respective plugins (`grpc_python_plugin`, `grpc_php_plugin`).
