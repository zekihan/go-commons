package oidcauth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/zekihan/go-commons/mockidp"
	"github.com/zekihan/go-commons/sqlitestore"
)

type testIdentity struct {
	Subject string
	Label   string
}

func openAuthDB(t *testing.T) *sqlitestore.Store {
	t.Helper()

	st, err := sqlitestore.Open(t.Context(), ":memory:", []string{Migration})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	return st
}

func newIDPService(t *testing.T) (*Service[testIdentity], *mockidp.Server) {
	t.Helper()

	idp, err := mockidp.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = idp.Close() })

	st := openAuthDB(t)

	service, err := New(t.Context(), &Config[testIdentity]{
		PublicURL:    "https://app.example",
		Issuer:       idp.Config.Issuer,
		ClientID:     idp.Config.ClientID,
		ClientSecret: idp.Config.ClientSecret,
		OwnerSubject: idp.Config.Subject,
		AppName:      "Test App",
		Authorize: func(info IdentityInfo) (testIdentity, bool) {
			return testIdentity{Subject: info.Subject, Label: "owner"}, info.Subject != ""
		},
	}, st.DB())
	if err != nil {
		t.Fatal(err)
	}

	return service, idp
}

func TestNewRequiresAuthorize(t *testing.T) {
	t.Parallel()

	_, err := New[IdentityInfo](t.Context(), &Config[IdentityInfo]{}, openAuthDB(t).DB())
	if err == nil {
		t.Fatal("New without Authorize succeeded")
	}
}

func TestIdentityInfoAuthorize(t *testing.T) {
	t.Parallel()

	info := IdentityInfo{Issuer: "https://issuer.example", Subject: "sub-1"}

	got, ok := IdentityInfoAuthorize(info)
	if !ok || got.Subject != "sub-1" {
		t.Fatalf("IdentityInfoAuthorize = %v, %v", got, ok)
	}

	if _, ok := IdentityInfoAuthorize(IdentityInfo{}); ok {
		t.Fatal("empty subject authorized")
	}
}

func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func doLoginFlow(t *testing.T, service *Service[testIdentity]) *http.Cookie {
	t.Helper()

	// Start sign-in.
	loginRec := httptest.NewRecorder()
	service.LoginHandler().ServeHTTP(loginRec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/auth/login", nil))

	if loginRec.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, body %s", loginRec.Code, loginRec.Body.String())
	}

	var txCookie *http.Cookie

	for _, cookie := range loginRec.Result().Cookies() {
		if cookie.Name == "oidc_tx" {
			txCookie = cookie
		}
	}

	if txCookie == nil {
		t.Fatal("login did not set transaction cookie")
	}

	// Ask the provider to authorize; it redirects to the callback with a code.
	authReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		loginRec.Header().Get("Location"), http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := noRedirectClient().Do(authReq)
	if err != nil {
		t.Fatal(err)
	}

	_ = resp.Body.Close()

	callbackURL, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}

	if callbackURL.Query().Get("code") == "" {
		t.Fatalf("authorize redirect has no code: %s", callbackURL)
	}

	// Complete sign-in at the callback.
	callbackReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/auth/callback?"+callbackURL.RawQuery, nil)
	callbackReq.AddCookie(txCookie)

	callbackRec := httptest.NewRecorder()
	service.CallbackHandler().ServeHTTP(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusSeeOther {
		t.Fatalf("callback status = %d, body %s", callbackRec.Code, callbackRec.Body.String())
	}

	if location := callbackRec.Header().Get("Location"); location != "/" {
		t.Fatalf("callback redirect = %q, want /", location)
	}

	for _, cookie := range callbackRec.Result().Cookies() {
		if cookie.Name == "session" {
			return cookie
		}
	}

	t.Fatal("callback did not set session cookie")

	return nil
}

func TestLoginSessionAndLogout(t *testing.T) {
	t.Parallel()

	service, idp := newIDPService(t)
	sessionCookie := doLoginFlow(t, service)

	// The session authenticates and exposes the mapped generic identity.
	var got testIdentity

	var gotOK bool

	protected := service.RequireSession(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, gotOK = Identity[testIdentity](r.Context())

			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api", nil)
	req.AddCookie(sessionCookie)

	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !gotOK || got.Subject != idp.Config.Subject ||
		got.Label != "owner" {
		t.Fatalf("protected = %d, identity %v (ok %v)", rec.Code, got, gotOK)
	}

	// CSRF token is tied to the session cookie.
	formReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/submit",
		strings.NewReader(url.Values{"csrf": {service.CSRFToken(req)}}.Encode()))
	formReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	formReq.AddCookie(sessionCookie)

	err := formReq.ParseForm()
	if err != nil {
		t.Fatal(err)
	}

	if !service.ValidCSRF(formReq) {
		t.Fatal("valid CSRF token rejected")
	}

	if service.CSRFToken(
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil),
	) != "" {
		t.Fatal("CSRF token without session cookie is not empty")
	}

	// Logout deletes the session.
	logoutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)

	logoutRec := httptest.NewRecorder()
	service.LogoutHandler().ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", logoutRec.Code)
	}

	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout status = %d, want 401", rec.Code)
	}
}

func TestRequireSessionRejectsMissingAndCallbackRejectsBadState(t *testing.T) {
	t.Parallel()

	service, _ := newIDPService(t)

	rec := httptest.NewRecorder()
	service.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session status = %d, want 401", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
		t.Fatalf("no session body = %s", rec.Body.String())
	}

	badReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/auth/callback?code=x&state=y", nil)
	//nolint:gosec // Test request cookie.
	badReq.AddCookie(&http.Cookie{Name: "oidc_tx", Value: "bogus"})

	badRec := httptest.NewRecorder()
	service.CallbackHandler().ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad callback status = %d, want 400", badRec.Code)
	}
}

// Bearer fixture: a local issuer serving discovery and JWKS for a test RSA key.

type bearerFixture struct {
	now          time.Time
	service      *Service[IdentityInfo]
	key          *rsa.PrivateKey
	issuer       string
	failNextJWKS atomic.Bool
}

const (
	testMCPClient   = "mcp-client"
	testMCPAudience = "https://app.example/mcp"
	testSubject     = "owner-subject"
)

func newBearerFixture(t *testing.T, client *http.Client) *bearerFixture {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	issuer := httptest.NewServer(mux)
	t.Cleanup(issuer.Close)

	fixture := &bearerFixture{
		key:    key,
		issuer: issuer.URL,
		now:    time.Now().Truncate(time.Second),
	}

	writeJSON := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json")

		encodeErr := json.NewEncoder(w).Encode(value)
		if encodeErr != nil {
			t.Error(encodeErr)
		}
	}

	mux.HandleFunc("/.well-known/openid-configuration",
		func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"issuer":                 issuer.URL,
				"authorization_endpoint": issuer.URL + "/authorize",
				"token_endpoint":         issuer.URL + "/token",
				"jwks_uri":               issuer.URL + "/jwks",
			})
		})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		if fixture.failNextJWKS.Swap(false) {
			http.Error(w, "Retry later", http.StatusServiceUnavailable)

			return
		}

		writeJSON(w, map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"kid": "k1",
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})

	service, err := New(t.Context(), &Config[IdentityInfo]{
		PublicURL:    "https://app.example",
		Issuer:       issuer.URL,
		ClientID:     "web-client",
		ClientSecret: "secret",
		OwnerSubject: testSubject,
		MCPClientID:  testMCPClient,
		MCPAudience:  testMCPAudience,
		Authorize:    IdentityInfoAuthorize,
		HTTPClient:   client,
	}, openAuthDB(t).DB())
	if err != nil {
		t.Fatal(err)
	}

	fixture.service = service
	service.now = func() time.Time { return fixture.now }

	return fixture
}

func TestDefaultHTTPClientAndJWKSRecovery(t *testing.T) {
	t.Parallel()

	fixture := newBearerFixture(t, nil)

	client := fixture.service.cfg.HTTPClient
	if client == nil || client == http.DefaultClient || client.Timeout != 12*time.Second {
		t.Fatalf("HTTP client = %+v, want dedicated client with a 12s timeout", client)
	}

	token := fixture.token(t, nil)
	fixture.failNextJWKS.Store(true)

	if rec := fixture.call(t, token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bearer status after JWKS failure = %d, want 401", rec.Code)
	}

	if rec := fixture.call(t, token); rec.Code != http.StatusOK {
		t.Fatalf("bearer status after JWKS recovery = %d, want 200", rec.Code)
	}
}

func TestNewPreservesCustomHTTPClient(t *testing.T) {
	t.Parallel()

	client := &http.Client{Timeout: time.Second}

	fixture := newBearerFixture(t, client)
	if got := fixture.service.cfg.HTTPClient; got != client || got.Timeout != time.Second {
		t.Fatalf("HTTP client = %+v, want original client with a 1s timeout", got)
	}
}

func (fixture *bearerFixture) token(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":       fixture.issuer,
		"sub":       testSubject,
		"aud":       []string{testMCPAudience},
		"exp":       fixture.now.Add(5 * time.Minute).Unix(),
		"iat":       fixture.now.Unix(),
		"nbf":       fixture.now.Unix(),
		"client_id": testMCPClient,
		"name":      "Owner",
		"email":     "owner@example.com",
	}
	if mutate != nil {
		mutate(claims)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["typ"] = "at+jwt"

	signed, err := token.SignedString(fixture.key)
	if err != nil {
		t.Fatal(err)
	}

	// jwt v5 omits kid unless set; the verifier does not require one here.
	return signed
}

func (fixture *bearerFixture) call(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	fixture.service.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := Identity[IdentityInfo](r.Context())
		if !ok || info.Subject != testSubject {
			t.Errorf("bearer identity = %v, ok %v", info, ok)
		}

		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	return rec
}

func TestRequireBearer(t *testing.T) {
	t.Parallel()

	fixture := newBearerFixture(t, nil)

	if rec := fixture.call(t, fixture.token(t, nil)); rec.Code != http.StatusOK {
		t.Fatalf("valid bearer status = %d, body %s", rec.Code, rec.Body.String())
	}

	cases := map[string]func(jwt.MapClaims){
		"wrong audience": func(c jwt.MapClaims) { c["aud"] = []string{"https://other.example"} },
		"wrong subject":  func(c jwt.MapClaims) { c["sub"] = "someone-else" },
		"wrong client":   func(c jwt.MapClaims) { c["client_id"] = "other-client" },
		"expired": func(c jwt.MapClaims) {
			c["exp"] = fixture.now.Add(-time.Minute).Unix()
			c["iat"] = fixture.now.Add(-time.Hour).Unix()
			c["nbf"] = fixture.now.Add(-time.Hour).Unix()
		},
	}

	for name, mutate := range cases {
		if rec := fixture.call(t, fixture.token(t, mutate)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
	}

	if rec := fixture.call(t, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", rec.Code)
	}

	if rec := fixture.call(t, "not-a-jwt"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("malformed token status = %d, want 401", rec.Code)
	}

	// Protected resource metadata is served while bearer auth is enabled.
	rec := httptest.NewRecorder()
	fixture.service.ProtectedResourceHandler().ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/.well-known/oauth-protected-resource", nil))

	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), fmt.Sprintf(`"resource":%q`, testMCPAudience)) {
		t.Fatalf("protected resource = %d %s", rec.Code, rec.Body.String())
	}

	// Context without the middleware carries no identity.
	if _, ok := Identity[IdentityInfo](t.Context()); ok {
		t.Fatal("identity present on empty context")
	}
}

func TestBearerDisabled(t *testing.T) {
	t.Parallel()

	service, _ := newIDPService(t)

	if service.MCPEnabled() {
		t.Fatal("MCP reported enabled without a client")
	}

	rec := httptest.NewRecorder()
	service.ProtectedResourceHandler().ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/.well-known/oauth-protected-resource", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled protected resource status = %d, want 404", rec.Code)
	}
}
