package store

import "context"

type tenantKey struct{}

// WithTenant returns a context carrying the given tenant id.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

// TenantFromContext returns the tenant id stored in the context, if any.
func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantKey{}).(string); ok {
		return v
	}
	return ""
}
