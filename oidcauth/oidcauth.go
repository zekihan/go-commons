// Package oidcauth implements SSO sign-in against one OIDC provider with
// server-side sessions, plus bearer-token protection for an MCP endpoint.
//
// The service is generic over the application's identity type: Config.Authorize
// maps the verified IdentityInfo (issuer, subject, name, email) to that type
// and decides whether it may sign in. Applications whose identity is just the
// verified claims can use IdentityInfo itself with IdentityInfoAuthorize.
//
// The database must already contain the auth tables; append Migration to the
// application's sqlitestore migrations. The database handle is shared, so
// session writes serialize with the application's own transactions.
package oidcauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jmoiron/sqlx"
	"golang.org/x/oauth2"
)

const (
	callbackPath = "/auth/callback"

	transactionLifetime = 10 * time.Minute
	sessionLifetime     = 30 * 24 * time.Hour
	renewalInterval     = time.Hour
	issuedAtSkew        = 5 * time.Minute

	// Fixed-width UTC timestamps sort chronologically in SQLite.
	timestampLayout = "2006-01-02T15:04:05.000000000Z"
)

// Migration creates the session and sign-in transaction tables. Append it to
// the application's migration list passed to sqlitestore.Open.
const Migration = `
	-- Server-side sessions; token_hash is the SHA-256 of the cookie value.
	CREATE TABLE sessions (
		token_hash TEXT PRIMARY KEY,
		issuer TEXT NOT NULL,
		subject TEXT NOT NULL,
		name TEXT,
		email TEXT,
		created_at TEXT NOT NULL,
		renewed_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);
	CREATE INDEX idx_sessions_expires ON sessions(expires_at);

	-- One-time OIDC sign-in transactions; id_hash is the SHA-256 of the
	-- transaction cookie value.
	CREATE TABLE oidc_transactions (
		id_hash TEXT PRIMARY KEY,
		state TEXT NOT NULL,
		nonce TEXT NOT NULL,
		pkce_verifier TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);
`

var (
	errNoJWKS            = errors.New("OIDC discovery document has no jwks_uri")
	errAuthorizeRequired = errors.New("oidcauth: Config.Authorize is required")
)

// IdentityInfo is the verified identity from an ID token, an access token,
// or a stored session, before the application maps it to its identity type.
type IdentityInfo struct {
	Name    *string `json:"name"`
	Email   *string `json:"email"`
	Issuer  string  `json:"-"`
	Subject string  `json:"subject"`
}

// AuthorizeFunc maps verified claims to the application identity type and
// reports whether that identity may proceed. It must derive the result only
// from the supplied claims: it is called again with the stored session claims
// on every authenticated request.
type AuthorizeFunc[T any] func(info IdentityInfo) (T, bool)

// IdentityInfoAuthorize authorizes the configured owner subject and returns
// the claims unchanged, for applications whose identity type is IdentityInfo.
func IdentityInfoAuthorize(info IdentityInfo) (IdentityInfo, bool) {
	return info, info.Subject != ""
}

// Config holds what the auth service needs from the server configuration.
type Config[T any] struct {
	// Authorize maps verified claims to the application identity. Required.
	Authorize AuthorizeFunc[T]
	// HTTPClient, when set, is used for provider discovery, key fetches and
	// the token exchange. Use it to bound those calls with a timeout.
	HTTPClient *http.Client
	PublicURL  string
	Issuer     string
	ClientID   string
	// ClientSecret is the OAuth client secret.
	ClientSecret string
	// OwnerSubject is the only subject allowed to sign in or present tokens.
	OwnerSubject string
	// MCPClientID is the OAuth client whose access tokens may call the MCP
	// endpoint; empty disables bearer protection.
	MCPClientID string
	// MCPAudience is the resource the access tokens must name in aud.
	MCPAudience string
	// AppName names the application in sign-in error pages. Default: a
	// generic "this service".
	AppName string
	// SessionCookie is the session cookie name. Default: "session".
	SessionCookie string
	// TransactionCookie is the sign-in transaction cookie name, scoped to the
	// callback path. Default: "oidc_tx".
	TransactionCookie string
	// SuccessPath is where a completed sign-in redirects. Default: "/".
	SuccessPath    string
	CookieInsecure bool
}

// Service owns the sign-in flow and sessions for identity type T.
type Service[T any] struct {
	db       *sqlx.DB
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
	bearer   *oidc.IDTokenVerifier
	now      func() time.Time
	cfg      Config[T]
}

// New discovers the provider and returns the service. The database must
// already be migrated (see Migration).
func New[T any](ctx context.Context, cfg *Config[T], db *sqlx.DB) (*Service[T], error) {
	if cfg.Authorize == nil {
		return nil, errAuthorizeRequired
	}

	resolved := *cfg
	if resolved.SessionCookie == "" {
		resolved.SessionCookie = "session"
	}

	if resolved.TransactionCookie == "" {
		resolved.TransactionCookie = "oidc_tx"
	}

	if resolved.SuccessPath == "" {
		resolved.SuccessPath = "/"
	}

	if resolved.HTTPClient != nil {
		ctx = oidc.ClientContext(ctx, resolved.HTTPClient)
	}

	provider, err := oidc.NewProvider(ctx, resolved.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}

	var discovery struct {
		JWKSURL string `json:"jwks_uri"` //nolint:tagliatelle // OIDC discovery field name.
	}

	err = provider.Claims(&discovery)
	if err != nil || discovery.JWKSURL == "" {
		return nil, errNoJWKS
	}

	// One key set serves both verifiers, so keys are fetched and cached once.
	keySet := oidc.NewRemoteKeySet(ctx, discovery.JWKSURL)

	service := &Service[T]{
		cfg:      resolved,
		db:       db,
		now:      time.Now,
		verifier: nil,
		bearer:   nil,
		oauth: &oauth2.Config{
			ClientID:     resolved.ClientID,
			ClientSecret: resolved.ClientSecret,
			RedirectURL:  strings.TrimRight(resolved.PublicURL, "/") + callbackPath,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}
	service.verifier = oidc.NewVerifier(resolved.Issuer, keySet, &oidc.Config{
		ClientID: resolved.ClientID,
		SupportedSigningAlgs: []string{
			oidc.RS256, oidc.RS384, oidc.RS512,
			oidc.ES256, oidc.ES384, oidc.ES512,
			oidc.PS256, oidc.PS384, oidc.PS512,
			oidc.EdDSA,
		},
		Now:                        func() time.Time { return service.now() },
		SkipClientIDCheck:          false,
		SkipExpiryCheck:            false,
		SkipIssuerCheck:            false,
		InsecureSkipSignatureCheck: false,
	})
	service.bearer = newBearerVerifier(
		resolved.Issuer,
		keySet,
		func() time.Time { return service.now() },
	)

	return service, nil
}

func (svc *Service[T]) isOwner(issuer, subject string) bool {
	return issuer == svc.cfg.Issuer && subject != "" && subject == svc.cfg.OwnerSubject
}

// authorized maps verified claims to the application identity, enforcing the
// configured owner subject first.
func (svc *Service[T]) authorized(info IdentityInfo) (T, bool) {
	var none T

	if !svc.isOwner(info.Issuer, info.Subject) {
		return none, false
	}

	identity, ok := svc.cfg.Authorize(info)
	if !ok {
		return none, false
	}

	return identity, true
}

func (svc *Service[T]) appName() string {
	if svc.cfg.AppName != "" {
		return svc.cfg.AppName
	}

	return "this service"
}

func randomToken() string {
	var random [32]byte
	rand.Read(random[:])

	return base64.RawURLEncoding.EncodeToString(random[:])
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

func timestamp(instant time.Time) string {
	return instant.UTC().Format(timestampLayout)
}

func (svc *Service[T]) setCookie(w http.ResponseWriter, name, path, value string, maxAge int) {
	//nolint:gosec,exhaustruct_v5 // Secure is configurable for local HTTP; host-only cookies.
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     path,
		Value:    value,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   !svc.cfg.CookieInsecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}

	w.Header().Set("Allow", method)
	http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)

	return false
}
