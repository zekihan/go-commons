# go-commons

Shared Go utilities for building HTTP services: secure cookies, sessions,
`net/http` middleware, pub/sub, response helpers, environment loading, and
assorted helpers. This is a pure library — there is no `main` package.

## Install

```bash
go get github.com/zekihan/go-commons
```

## Packages

- **`cookie`** — Secure cookie encode/decode via `gorilla/securecookie`. A
  singleton `SecureCookie` is initialized once via `sync.Once`. Call
  `InitSecureCookie(hashKey, blockKey)` at startup with stable keys; omitting it
  auto-generates random keys (which are useless across restarts).
- **`session`** — `Cookie` struct representing a session (ID, created/expiry
  times, authenticated flag), used by `middleware` to validate auth.
- **`middleware`** — Standard `net/http` middleware: cookie-based auth
  (`GetAuthMiddlewareForCookie`), API key auth (`GetAuthMiddlewareForSingleAPIKey`,
  `GetAuthMiddlewareWithFunction`), structured request logging, and panic
  recovery. HTMX requests in the auth middleware get `HX-Refresh` instead of a
  redirect.
- **`pubsub`** — `UniversalPubSub` wraps either an in-memory `LocalPubSub` or a
  `RedisPubSub`, selected by `NewUniversalPubSub(opts)` based on whether
  `opts.Redis` is set. Channel names are prefixed with `opts.Prefix` + `:` when
  non-empty.
- **`web`** — HTTP response helpers (`Write`, `WriteString`, `CheckError`) plus
  `GetAcceptType`. `CheckError` logs via `slog` and writes `http.Error`. Ready-made
  handlers live in `web/ping`, `web/health`, `web/ready`, `web/favicon`, and
  `web/errorroutes`.
- **`environment`** — Loads `.env`, then `.override.env`, then any extra files via
  `godotenv.Overload`.
- **`utils`** — Miscellaneous helpers: `InitLogger` (tint-colored `slog`),
  `WrapErrors`, `RandString`, generics (`Contains`, `Filter`, `PointerTo`), IP
  extraction, HTML/JS minification, version info, and command execution.

## Development

```bash
make tidy    # fmt + go mod tidy
make lint    # tidy + golangci-lint run --fix
make test    # run all tests with -race
make audit   # vet + staticcheck + govulncheck + tests
```
