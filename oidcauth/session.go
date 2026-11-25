package oidcauth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type identityKey[T any] struct{}

func (svc *Service[T]) createSession(
	ctx context.Context,
	w http.ResponseWriter,
	info IdentityInfo,
) bool {
	value := randomToken()
	now := svc.now()
	created := timestamp(now)

	_, err := svc.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, issuer, subject, name, email, created_at, renewed_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tokenHash(value), svc.cfg.Issuer, info.Subject, info.Name, info.Email,
		created, created, timestamp(now.Add(sessionLifetime)),
	)
	if err != nil {
		return false
	}

	svc.setCookie(w, svc.cfg.SessionCookie, "/", value, int(sessionLifetime/time.Second))

	return true
}

// LogoutHandler serves POST /auth/logout. It needs no valid session.
func (svc *Service[T]) LogoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}

		cookie, err := r.Cookie(svc.cfg.SessionCookie)
		if err == nil && cookie.Value != "" {
			_, err = svc.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash = ?",
				tokenHash(cookie.Value))
			if err != nil {
				slog.ErrorContext(r.Context(), "Could not delete session on sign-out")
			}
		}

		svc.setCookie(w, svc.cfg.SessionCookie, "/", "", -1)
		w.WriteHeader(http.StatusNoContent)
	})
}

// RequireSession rejects requests without a valid session with 401 JSON and
// renews the session when due. The identity is available via Identity.
func (svc *Service[T]) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		cookie, err := r.Cookie(svc.cfg.SessionCookie)
		if err != nil || cookie.Value == "" {
			unauthenticated(w)

			return
		}

		ctx := r.Context()
		now := svc.now()
		hash := tokenHash(cookie.Value)

		var (
			info            IdentityInfo
			issuer, renewed string
		)

		err = svc.db.QueryRowxContext(ctx, `
			SELECT issuer, subject, name, email, renewed_at FROM sessions
			WHERE token_hash = ? AND expires_at > ?`, hash, timestamp(now),
		).Scan(&issuer, &info.Subject, &info.Name, &info.Email, &renewed)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			sessionError(w)

			return
		}

		info.Issuer = issuer

		identity, ok := svc.authorized(info)
		if err != nil || !ok {
			unauthenticated(w)

			return
		}

		if renewed < timestamp(now.Add(-renewalInterval)) {
			didRenew, err := svc.renewSession(ctx, now, hash, renewed)
			if err != nil {
				sessionError(w)

				return
			}

			if didRenew {
				svc.setCookie(w, svc.cfg.SessionCookie, "/", cookie.Value,
					int(sessionLifetime/time.Second))
			}
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, identityKey[T]{}, identity)))
	})
}

func (svc *Service[T]) renewSession(
	ctx context.Context,
	now time.Time,
	hash, renewed string,
) (bool, error) {
	// Match the observed renewal so concurrent requests extend the session only once.
	result, err := svc.db.ExecContext(ctx, `
		UPDATE sessions SET renewed_at = ?, expires_at = ?
		WHERE token_hash = ? AND renewed_at = ? AND expires_at > ?`,
		timestamp(now), timestamp(now.Add(sessionLifetime)), hash, renewed, timestamp(now),
	)
	if err != nil {
		return false, fmt.Errorf("renew session: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count renewed sessions: %w", err)
	}

	return count == 1, nil
}

// CSRFToken returns the CSRF token for the session cookie on r, derived from
// the session token so forms can prove they were rendered for this session.
// It is empty without a session cookie.
func (svc *Service[T]) CSRFToken(r *http.Request) string {
	cookie, err := r.Cookie(svc.cfg.SessionCookie)
	if err != nil || cookie.Value == "" {
		return ""
	}

	return tokenHash("csrf:" + cookie.Value)
}

// ValidCSRF reports whether the submitted "csrf" form field matches the
// session cookie on r, compared in constant time.
func (svc *Service[T]) ValidCSRF(r *http.Request) bool {
	want := svc.CSRFToken(r)

	return want != "" &&
		subtle.ConstantTimeCompare([]byte(want), []byte(r.PostForm.Get("csrf"))) == 1
}

func unauthenticated(w http.ResponseWriter) {
	writeSessionError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
}

func sessionError(w http.ResponseWriter) {
	writeSessionError(w, http.StatusInternalServerError, "internal", "Could not check the session.")
}

func writeSessionError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	//nolint:errcheck,errchkjson // A write failure cannot change the response after headers are sent.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// Identity returns the identity stored on the request context by
// RequireSession or RequireBearer for identity type T.
func Identity[T any](ctx context.Context) (T, bool) {
	identity, ok := ctx.Value(identityKey[T]{}).(T)

	return identity, ok
}
