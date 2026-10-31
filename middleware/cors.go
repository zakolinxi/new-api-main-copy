package middleware

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS builds the shared CORS policy.
//
// Security note: gin-contrib/cors emits `Access-Control-Allow-Origin: *` when
// AllowAllOrigins is set, and browsers reject `*` for credentialed requests.
// The previous configuration combined AllowAllOrigins with AllowCredentials,
// which is self-contradictory: it advertised intent to allow cross-origin
// credentialed reads while the emitted `*` made that impossible. Worse, any
// later "fix" that swapped AllowAllOrigins for a reflecting AllowOriginFunc
// would have turned it into a real cross-origin credential leak.
//
// The policy now has two explicit, mutually exclusive modes:
//
//  1. SESSION_COOKIE_TRUSTED_URL configured (dashboard served from a different
//     origin behind a trusted reverse proxy): only those exact origins are
//     allowed, and credentials are enabled because SameSite=Strict cookies are
//     expected to flow.
//  2. No trusted origin configured (default): any origin may call the API
//     without credentials, which is safe because dashboard authentication uses
//     an explicit Authorization bearer token rather than ambient cookies.
//
// Cross-site session cookies are SameSite=Strict, so mode 2 never silently
// allows credential-bearing cross-site requests.
func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	// Keep the wildcard for request headers: these are request-side only and do
	// not relax the response-side origin policy.
	config.AllowHeaders = []string{"*"}

	trustedOrigins := normalizeCORSOrigins(common.SessionCookieTrustedURLs)
	if len(trustedOrigins) == 0 {
		// Default mode: any origin, but never with ambient credentials.
		config.AllowAllOrigins = true
		config.AllowCredentials = false
		return cors.New(config)
	}

	// Trusted-proxy mode: exact origin allowlist plus credentialed requests.
	config.AllowAllOrigins = false
	config.AllowCredentials = true
	config.AllowOriginFunc = func(origin string) bool {
		normalized := strings.TrimSpace(origin)
		if normalized == "" {
			return false
		}
		// Compare canonically so that `https://Example.com:443` and
		// `https://example.com` are treated identically, without ever
		// reflecting an attacker-controlled string back verbatim.
		canonical, err := common.NormalizeOrigin(normalized)
		if err != nil {
			return false
		}
		return slices.Contains(trustedOrigins, canonical)
	}
	return cors.New(config)
}

// normalizeCORSOrigins canonicalizes configured origins and drops unusable
// entries. Invalid entries are ignored rather than aborting startup because
// InitSessionCookieSettings already rejects malformed env values.
func normalizeCORSOrigins(origins []string) []string {
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		canonical, err := common.NormalizeOrigin(origin)
		if err != nil {
			continue
		}
		if !slices.Contains(normalized, canonical) {
			normalized = append(normalized, canonical)
		}
	}
	return normalized
}

func Version() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
