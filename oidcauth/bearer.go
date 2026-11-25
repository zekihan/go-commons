package oidcauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	protectedResourcePath = "/.well-known/oauth-protected-resource"
	bearerPrefix          = "Bearer "
)

// ProtectedResource is the OAuth 2.0 protected resource metadata (RFC 9728)
// for the bearer-protected endpoint.
//
//nolint:tagliatelle // RFC 9728 field names.
type ProtectedResource struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported"`
}

// newBearerVerifier checks signature, iss, exp and nbf for access tokens. The
// audience, client and token type checks are in verifyAccessToken.
func newBearerVerifier(
	issuer string,
	keySet oidc.KeySet,
	now func() time.Time,
) *oidc.IDTokenVerifier {
	return oidc.NewVerifier(issuer, keySet, &oidc.Config{
		ClientID:                   "",
		SupportedSigningAlgs:       []string{oidc.RS256},
		Now:                        now,
		SkipClientIDCheck:          true,
		SkipExpiryCheck:            false,
		SkipIssuerCheck:            false,
		InsecureSkipSignatureCheck: false,
	})
}

// MCPEnabled reports whether a bearer client is configured.
func (svc *Service[T]) MCPEnabled() bool {
	return svc.cfg.MCPClientID != ""
}

// ProtectedResourceHandler serves the protected resource metadata.
func (svc *Service[T]) ProtectedResourceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !svc.MCPEnabled() {
			notFound(w)

			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck // A write failure cannot change the response after headers are sent.
		_ = json.NewEncoder(w).Encode(ProtectedResource{
			Resource:               svc.cfg.MCPAudience,
			AuthorizationServers:   []string{svc.cfg.Issuer},
			BearerMethodsSupported: []string{"header"},
			ScopesSupported:        []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
		})
	})
}

// RequireBearer accepts only Authorization: Bearer access tokens issued to
// the configured client for the configured audience and owner. The identity
// is available via Identity. It responds 404 when bearer auth is disabled.
func (svc *Service[T]) RequireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !svc.MCPEnabled() {
			notFound(w)

			return
		}

		w.Header().Set("Cache-Control", "no-store")

		if r.URL.Query().Has("access_token") {
			svc.bearerChallenge(w, "invalid_request",
				"Send the access token in the Authorization header.")

			return
		}

		values := r.Header.Values("Authorization")
		if len(values) == 0 {
			svc.bearerChallenge(w, "", "An access token is required.")

			return
		}

		raw, ok := bearerToken(values)
		if !ok {
			svc.bearerChallenge(w, "invalid_request", "The Authorization header is malformed.")

			return
		}

		var identity T

		if ok = jwtShaped(raw); ok {
			identity, ok = svc.verifyAccessToken(r.Context(), raw)
		}

		if !ok {
			svc.bearerChallenge(w, "invalid_token", "The access token is not valid.")

			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey[T]{}, identity)))
	})
}

func bearerToken(values []string) (string, bool) {
	if len(values) != 1 || len(values[0]) <= len(bearerPrefix) ||
		!strings.EqualFold(values[0][:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	return values[0][len(bearerPrefix):], true
}

func jwtShaped(token string) bool {
	return strings.Count(token, ".") == 2 && !strings.ContainsFunc(token, func(c rune) bool {
		return !isBase64URL(c) && c != '.'
	})
}

func isBase64URL(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

type accessHeader struct {
	Typ *string `json:"typ"`
	Alg string  `json:"alg"`
}

//nolint:tagliatelle // RFC 9068 claim names.
type accessClaims struct {
	Nonce    json.RawMessage `json:"nonce"`
	ClientID json.RawMessage `json:"client_id"`
	AZP      json.RawMessage `json:"azp"`
	Name     json.RawMessage `json:"name"`
	Email    json.RawMessage `json:"email"`
}

func (svc *Service[T]) verifyAccessToken(ctx context.Context, raw string) (T, bool) {
	var none T

	if !validAccessHeader(raw) {
		return none, false
	}

	token, err := svc.bearer.Verify(ctx, raw)
	if err != nil || token.Expiry.IsZero() || token.IssuedAt.IsZero() ||
		token.IssuedAt.After(svc.now().Add(issuedAtSkew)) ||
		!slices.Contains(token.Audience, svc.cfg.MCPAudience) {
		return none, false
	}

	var claims accessClaims
	if token.Claims(&claims) != nil || claims.Nonce != nil || !svc.validMCPClient(&claims) {
		return none, false
	}

	return svc.authorized(IdentityInfo{
		Issuer:  token.Issuer,
		Subject: token.Subject,
		Name:    optionalString(claims.Name),
		Email:   optionalString(claims.Email),
	})
}

// validAccessHeader allows only RS256 and rejects tokens typed as anything
// other than an RFC 9068 access token, such as ID tokens.
func validAccessHeader(raw string) bool {
	encoded, _, _ := strings.Cut(raw, ".")

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}

	var header accessHeader
	if json.Unmarshal(decoded, &header) != nil || header.Alg != oidc.RS256 {
		return false
	}

	return header.Typ == nil || strings.EqualFold(*header.Typ, "at+jwt") ||
		strings.EqualFold(*header.Typ, "application/at+jwt")
}

// validMCPClient matches client_id (RFC 9068), or azp when client_id is absent.
func (svc *Service[T]) validMCPClient(claims *accessClaims) bool {
	raw := claims.ClientID
	if raw == nil {
		raw = claims.AZP
	}

	client := optionalString(raw)

	return client != nil && *client == svc.cfg.MCPClientID
}

func optionalString(raw json.RawMessage) *string {
	var value *string
	if raw == nil || json.Unmarshal(raw, &value) != nil || value == nil {
		return nil
	}

	return value
}

func (svc *Service[T]) bearerChallenge(w http.ResponseWriter, code, message string) {
	challenge := `Bearer resource_metadata="` + strings.TrimRight(svc.cfg.PublicURL, "/") +
		protectedResourcePath + `"`
	if code != "" {
		challenge += `, error="` + code + `"`
	}

	w.Header().Set("WWW-Authenticate", challenge)
	writeSessionError(w, http.StatusUnauthorized, "unauthenticated", message)
}

func notFound(w http.ResponseWriter) {
	writeSessionError(w, http.StatusNotFound, "not_found", "Not found.")
}
