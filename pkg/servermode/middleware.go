package servermode

import (
	"net"
	"net/http"
	"strings"
)

const cookieName = "pmanage_token"

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if t := r.Header.Get("X-PManage-Token"); t != "" {
		return t
	}
	return r.URL.Query().Get("token")
}

// Middleware gates server-mode HTTP traffic with the configured read token.
//
// Token delivery: the operator first opens `http://host:port/?token=SECRET`.
// That request (asset page or API probe) is validated, a SameSite httpOnly
// cookie is set, and every subsequent request is authorized by the cookie.
//
// Path /health stays open for load balancers. When no token is configured and
// the server bound localhost, enforcement is skipped (loopback trusted).
func (c *Config) Middleware(next http.Handler) http.Handler {
	if c == nil {
		return next
	}
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// /health is deliberately unauthenticated (LB/uptime probes).
		if r.URL.Path == "/health" {
			next.ServeHTTP(rw, r)
			return
		}

		// Loopback with no configured token: trusted.
		if c.Token == "" && c.isLocalhost() {
			next.ServeHTTP(rw, r)
			return
		}

		// Always trust a previously-issued cookie.
		if ck, err := r.Cookie(cookieName); err == nil && c.ValidToken(ck.Value) {
			next.ServeHTTP(rw, r)
			return
		}

		if tok := bearerToken(r); c.ValidToken(tok) {
			// Fresh handshake: mint the cookie for the duration of the visit.
			http.SetCookie(rw, &http.Cookie{
				Name:     cookieName,
				Value:    c.Token,
				Path:     "/",
				SameSite: http.SameSiteStrictMode,
				HttpOnly: true,
				Secure:   r.TLS != nil,
			})
			next.ServeHTTP(rw, r)
			return
		}

		http.Error(rw, "unauthorized: missing or invalid token", http.StatusUnauthorized)
	})
}

// SetTokenCookie applies the session cookie manually (e.g. from a settings
// dialog when the browser reached the UI without a ?token= handshake).
func SetTokenCookie(rw http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(rw, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		HttpOnly: true,
		Secure:   r.TLS != nil,
	})
}

// ClientRemoteAddr returns the client's IP.
func ClientRemoteAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}