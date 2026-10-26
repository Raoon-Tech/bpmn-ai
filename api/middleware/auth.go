package middleware

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// AuthConfig configures the authentication middleware.
type AuthConfig struct {
	Mode             string
	JWKSURL          string
	Issuer           string
	Audience         string
	UserClaim        string
	RolesClaim       string
	TenantClaim      string
	TrustedProxies   []string
	EnforceTaskRoles bool
}

// Identity is the resolved caller identity.
type Identity struct {
	UserID   string
	Roles    []string
	TenantID string
}

type identityKey struct{}

// WithIdentity stores an identity in the context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFromContext returns the identity stored in the context, if any.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// Auth returns a middleware that resolves the caller identity according to cfg.
func Auth(cfg AuthConfig) func(http.Handler) http.Handler {
	if cfg.UserClaim == "" {
		cfg.UserClaim = "sub"
	}

	if cfg.Mode == "jwt" && cfg.JWKSURL == "" {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeAuthError(w, http.StatusInternalServerError, "jwks not configured")
			})
		}
	}
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "roles"
	}
	if cfg.TenantClaim == "" {
		cfg.TenantClaim = "tenant_id"
	}

	var jwks keyfunc.Keyfunc
	var jwksErr error
	if cfg.Mode == "jwt" && cfg.JWKSURL != "" {
		jwks, jwksErr = keyfunc.NewDefault([]string{cfg.JWKSURL})
	}

	trustedNets := parseCIDRs(cfg.TrustedProxies)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch cfg.Mode {
			case "none", "":
				id := Identity{UserID: "anonymous"}
				ctx := WithIdentity(r.Context(), id)
				ctx = store.WithTenant(ctx, id.TenantID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return

			case "gateway":
				if !isTrustedProxy(r.RemoteAddr, trustedNets) {
					writeAuthError(w, http.StatusForbidden, "untrusted proxy")
					return
				}
				id := Identity{
					UserID:   r.Header.Get("X-User-Id"),
					TenantID: r.Header.Get("X-Tenant-Id"),
				}
				if roles := r.Header.Get("X-User-Roles"); roles != "" {
					id.Roles = splitRoles(roles)
				}
				if id.UserID == "" {
					writeAuthError(w, http.StatusUnauthorized, "missing X-User-Id")
					return
				}
				ctx := WithIdentity(r.Context(), id)
				ctx = store.WithTenant(ctx, id.TenantID)
				next.ServeHTTP(w, r.WithContext(ctx))

			case "jwt":
				if jwksErr != nil {
					writeAuthError(w, http.StatusInternalServerError, "jwks init failed")
					return
				}
				tokenStr := bearerToken(r)
				if tokenStr == "" {
					writeAuthError(w, http.StatusUnauthorized, "missing bearer token")
					return
				}
				parserOpts := []jwt.ParserOption{}
				if cfg.Issuer != "" {
					parserOpts = append(parserOpts, jwt.WithIssuer(cfg.Issuer))
				}
				if cfg.Audience != "" {
					parserOpts = append(parserOpts, jwt.WithAudience(cfg.Audience))
				}
				token, err := jwt.Parse(tokenStr, jwks.Keyfunc, parserOpts...)
				if err != nil || !token.Valid {
					writeAuthError(w, http.StatusUnauthorized, "invalid token")
					return
				}
				claims, _ := token.Claims.(jwt.MapClaims)
				id := Identity{
					UserID:   claimString(claims, cfg.UserClaim),
					TenantID: claimString(claims, cfg.TenantClaim),
				}
				if roles, ok := claims[cfg.RolesClaim]; ok {
					id.Roles = claimStrings(roles)
				}
				if id.UserID == "" {
					writeAuthError(w, http.StatusUnauthorized, "missing user claim")
					return
				}
				ctx := WithIdentity(r.Context(), id)
				ctx = store.WithTenant(ctx, id.TenantID)
				next.ServeHTTP(w, r.WithContext(ctx))

			default:
				writeAuthError(w, http.StatusInternalServerError, "unknown auth mode")
			}
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(h[len("Bearer "):])
}

func claimString(claims jwt.MapClaims, key string) string {
	if v, ok := claims[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func claimStrings(v any) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return val
	}
	return nil
}

func splitRoles(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseCIDRs(items []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, item := range items {
		if _, ipnet, err := net.ParseCIDR(item); err == nil {
			nets = append(nets, ipnet)
			continue
		}
		if ip := net.ParseIP(item); ip != nil {
			mask := net.CIDRMask(32, 32)
			if ip.To4() == nil {
				mask = net.CIDRMask(128, 128)
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: mask})
		}
	}
	return nets
}

func isTrustedProxy(remoteAddr string, nets []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if len(nets) == 0 {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
