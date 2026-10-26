# Autenticación y delegación de identidad

El motor **no gestiona usuarios ni roles**. La identidad la resuelve el host y se inyecta en el contexto de la petición.

## Modos (`AUTH_MODE`)

| Modo | Descripción |
|---|---|
| `none` | Identidad anónima (desarrollo). |
| `jwt` | Valida un token Bearer contra un endpoint JWKS. |
| `gateway` | Confía en cabeceras de identidad de un reverse proxy. |

## Modo JWT

```bash
AUTH_MODE=jwt
AUTH_JWKS_URL=https://idp.example.com/.well-known/jwks.json
AUTH_ISSUER=https://idp.example.com/
AUTH_AUDIENCE=bpmn-engine
AUTH_USER_CLAIM=sub
AUTH_ROLES_CLAIM=roles
AUTH_TENANT_CLAIM=tenant_id
```

El token debe contener el claim de usuario. Roles y tenant son opcionales.

## Modo gateway

```bash
AUTH_MODE=gateway
AUTH_TRUSTED_PROXIES=10.0.0.0/8,127.0.0.1
```

El reverse proxy establece `X-User-Id`, `X-User-Roles` (separados por coma) y `X-Tenant-Id`. Las peticiones desde IPs no confiables se rechazan.

## Proveedor de identidad custom

El host puede inyectar su propio resolver:

```go
srv := http.NewServer(cfg, store, eng, q, logger, metrics,
    http.WithIdentityProvider(myProvider))
```

## Aplicación de roles en tareas

Con `AUTH_ENFORCE_TASK_ROLES=true`, el motor exige que los roles del llamante intersecten con `candidateGroups` de la tarea. Por defecto no se aplica autorización a nivel de tarea.
