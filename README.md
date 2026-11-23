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
- **`metrics`** — Prometheus-compatible metrics backed by
  `VictoriaMetrics/metrics`: a `/metrics` handler with Go runtime and process
  metrics, HTTP middleware labelled by method, route pattern, and status, and
  app-specific counters, gauges, and histograms.
- **`web`** — HTTP response helpers (`Write`, `WriteString`, `CheckError`) plus
  `GetAcceptType`. `CheckError` logs via `slog` and writes `http.Error`. Ready-made
  handlers live in `web/ping`, `web/health`, `web/ready`, `web/favicon`, and
  `web/errorroutes`.
- **`environment`** — Parses `.env`, `.override.env`, and any extra files with
  `godotenv`. Values override the process environment and later files override
  earlier files. Missing files and directories are skipped; an error stops loading
  without undoing values from earlier files.
- **`utils`** — Miscellaneous helpers: `InitLogger` (tint-colored `slog`),
  `WrapErrors`, `RandString`, generics (`Contains`, `Filter`, `PointerTo`), IP
  extraction, HTML/JS minification, version info, and command execution.

`utils.GetIP` trusts `X-Forwarded-For` and `X-Real-IP` before the remote
address. Ensure a trusted proxy controls these headers before using the result
for access control or other security decisions.

## Development

```bash
make tidy    # fmt + go mod tidy
make lint    # tidy + golangci-lint run --fix
make test    # run all tests with -race
make audit   # vet + staticcheck + govulncheck + tests
```

Publishing and manual recovery: [Publishing](docs/publishing.md).

## Metrics

```go
registry := metrics.New()

mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", handler)

// Route label is the ServeMux pattern ("/users/{id}"), never the raw path.
srv := &http.Server{Handler: registry.Middleware(mux)}

// App-specific metrics; labels must stay bounded (no IDs or raw paths).
jobs := registry.Counter("app_jobs_total", metrics.Labels{"kind": "sync"})
jobs.Inc()
registry.Gauge("app_queue_depth", nil).Set(3)

// Serve /metrics on an internal-only address, not via public ingress.
go registry.ListenAndServe(ctx, ":9090")
```

Mount `registry.Handler()` on the main mux instead if a separate listener is
not wanted. For non-ServeMux routing use `metrics.MiddlewareWithRoute` or
`metrics.WithRoute` to supply a bounded route name.

In Kubernetes, annotate the pod for vmagent scraping on the metrics port and
path `/metrics`.
