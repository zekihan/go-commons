package oidcauth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// LoginHandler serves GET /auth/login.
func (svc *Service[T]) LoginHandler() http.Handler {
	return http.HandlerFunc(svc.login)
}

func (svc *Service[T]) login(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	ctx := r.Context()
	now := svc.now()
	svc.deleteExpired(ctx, timestamp(now))

	value, state, nonce := randomToken(), randomToken(), randomToken()
	verifier := oauth2.GenerateVerifier()

	_, err := svc.db.ExecContext(ctx, `
		INSERT INTO oidc_transactions (id_hash, state, nonce, pkce_verifier, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		tokenHash(value), state, nonce, verifier, timestamp(now.Add(transactionLifetime)),
	)
	if err != nil {
		svc.signInError(w, http.StatusInternalServerError, "Could not start sign-in.")

		return
	}

	w.Header().Set("Cache-Control", "no-store")
	svc.setCookie(w, svc.cfg.TransactionCookie, callbackPath, value,
		int(transactionLifetime/time.Second))
	authURL := svc.oauth.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oidc.Nonce(nonce),
		oauth2.SetAuthURLParam("response_mode", "query"),
	)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

func (svc *Service[T]) deleteExpired(ctx context.Context, now string) {
	_, err := svc.db.ExecContext(ctx, "DELETE FROM oidc_transactions WHERE expires_at <= ?", now)
	if err != nil {
		slog.ErrorContext(ctx, "Could not delete expired sign-in transactions")
	}

	_, err = svc.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", now)
	if err != nil {
		slog.ErrorContext(ctx, "Could not delete expired sessions")
	}
}

// CallbackHandler serves GET /auth/callback.
func (svc *Service[T]) CallbackHandler() http.Handler {
	return http.HandlerFunc(svc.callback)
}

func (svc *Service[T]) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	svc.setCookie(w, svc.cfg.TransactionCookie, callbackPath, "", -1)

	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	cookie, err := r.Cookie(svc.cfg.TransactionCookie)
	if err != nil || cookie.Value == "" {
		invalidSignIn(w, svc)

		return
	}

	ctx := r.Context()

	var state, nonce, verifier, expires string

	err = svc.db.QueryRowxContext(ctx, `
		DELETE FROM oidc_transactions WHERE id_hash = ?
		RETURNING state, nonce, pkce_verifier, expires_at`, tokenHash(cookie.Value),
	).Scan(&state, &nonce, &verifier, &expires)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		svc.signInError(w, http.StatusInternalServerError, "Could not complete sign-in.")

		return
	}

	query := r.URL.Query()

	code := query.Get("code")
	if err != nil || expires <= timestamp(svc.now()) || query.Has("error") || code == "" ||
		subtle.ConstantTimeCompare([]byte(state), []byte(query.Get("state"))) != 1 {
		invalidSignIn(w, svc)

		return
	}

	token, claims := svc.exchange(ctx, code, verifier, nonce)
	if token == nil {
		invalidSignIn(w, svc)

		return
	}

	info := IdentityInfo{
		Issuer:  token.Issuer,
		Subject: token.Subject,
		Name:    claims.Name,
		Email:   claims.Email,
	}
	if _, ok := svc.authorized(info); !ok {
		svc.signInError(w, http.StatusForbidden,
			"This account cannot access "+svc.appName()+".")

		return
	}

	if !svc.createSession(ctx, w, info) {
		svc.signInError(w, http.StatusInternalServerError, "Could not create a session.")

		return
	}

	http.Redirect(w, r, svc.cfg.SuccessPath, http.StatusSeeOther)
}

type idClaims struct {
	Name  *string         `json:"name"`
	Email *string         `json:"email"`
	AZP   json.RawMessage `json:"azp"`
}

func (svc *Service[T]) exchange(
	ctx context.Context,
	code, verifier, nonce string,
) (*oidc.IDToken, *idClaims) {
	if svc.cfg.HTTPClient != nil {
		ctx = oidc.ClientContext(ctx, svc.cfg.HTTPClient)
	}

	oauthToken, err := svc.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, nil
	}

	raw, ok := oauthToken.Extra("id_token").(string)
	if !ok {
		return nil, nil
	}

	token, err := svc.verifier.Verify(ctx, raw)
	if err != nil || token.IssuedAt.IsZero() || token.IssuedAt.After(svc.now().Add(issuedAtSkew)) ||
		subtle.ConstantTimeCompare([]byte(token.Nonce), []byte(nonce)) != 1 {
		return nil, nil
	}

	var claims idClaims
	if token.Claims(&claims) != nil || !svc.validAuthorizedParty(token.Audience, claims.AZP) {
		return nil, nil
	}

	return token, &claims
}

func (svc *Service[T]) validAuthorizedParty(audience []string, raw json.RawMessage) bool {
	if raw == nil {
		return len(audience) == 1
	}

	var azp *string

	return json.Unmarshal(raw, &azp) == nil && azp != nil && *azp == svc.cfg.ClientID
}

func invalidSignIn[T any](w http.ResponseWriter, svc *Service[T]) {
	svc.signInError(w, http.StatusBadRequest, "Sign-in could not be verified.")
}

func (svc *Service[T]) signInError(w http.ResponseWriter, status int, message string) {
	title := "Sign-in"
	if svc.cfg.AppName != "" {
		title = svc.cfg.AppName + " sign-in"
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	//nolint:errcheck // A write failure cannot change the response after headers are sent.
	_, _ = w.Write([]byte(`<!doctype html><title>` + title + `</title><p>` +
		message + `</p><p><a href="/auth/login">Sign in again</a></p>`))
}
