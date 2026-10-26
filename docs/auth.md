# Authentication and identity delegation

The engine **does not manage users or roles**. Identity is resolved by the host and injected into the request context.

## Modes (`AUTH_MODE`)

| Mode | Description |
|---|---|
| `none` | Anonymous identity (development). |
| `jwt` | Validates a Bearer token against a JWKS endpoint. |
| `gateway` | Trusts identity headers from a reverse proxy. |

## JWT mode

```bash
AUTH_MODE=jwt
AUTH_JWKS_URL=https://idp.example.com/.well-known/jwks.json
AUTH_ISSUER=https://idp.example.com/
AUTH_AUDIENCE=bpmn-engine
AUTH_USER_CLAIM=sub          # default
AUTH_ROLES_CLAIM=roles       # default
AUTH_TENANT_CLAIM=tenant_id  # default
```

The token must contain the user claim. Roles and tenant are optional.

## Gateway mode

```bash
AUTH_MODE=gateway
AUTH_TRUSTED_PROXIES=10.0.0.0/8,127.0.0.1
```

The reverse proxy sets `X-User-Id`, `X-User-Roles` (comma separated) and `X-Tenant-Id`. Requests from non-trusted IPs are rejected.

## Custom identity provider

Hosts can inject their own resolver:

```go
srv := http.NewServer(cfg, store, eng, q, logger, metrics,
    http.WithIdentityProvider(myProvider))
```

## Task role enforcement

Set `AUTH_ENFORCE_TASK_ROLES=true` to require that the caller's roles intersect the task's `candidateGroups`. By default the engine does not enforce task-level authorization.
