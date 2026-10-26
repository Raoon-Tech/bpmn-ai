# Multi-tenant

Cada instancia de proceso puede asociarse a un tenant. El aislamiento se aplica en el store: `ListInstances` y `GetInstance` filtran por el tenant presente en el contexto de la petición.

## Cómo se establece el tenant

- **JWT**: claim configurado en `AUTH_TENANT_CLAIM` (default `tenant_id`).
- **Gateway**: cabecera `X-Tenant-Id`.
- **Código**: `store.WithTenant(ctx, tenantID)`.

## Almacenamiento

La columna `tenant_id` en `instances` (migración `0004_tenant_id`). Un valor vacío significa instancia global (sin tenant).

## Notas

- Las definiciones de proceso (`processes`) son compartidas entre tenants.
- Los jobs y logs no filtran por tenant por defecto; el tenant se recupera desde la instancia asociada.
- Para un aislamiento completo a nivel de base de datos, usar una BD por tenant (configuración externa).
