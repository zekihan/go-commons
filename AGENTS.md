# AGENTS.md

This file provides guidance to AI Agents when working with code in this repository.

## Commands

```bash
make tidy          # fmt + go mod tidy
make lint          # tidy + golangci-lint run --fix
make test          # run all tests with -race
make test/cover    # run tests and open coverage report in browser
make audit         # vet + staticcheck + govulncheck + tests
make new_version VERSION=x.y.z  # bump version
make release       # goreleaser release
```

To run a single test:
```bash
./scripts/go_test.sh -v -run TestFunctionName ./pkg/...
```

The test script sets `CONFIG_DIR` to the project root before running.

## Architecture

This is a pure Go library (no `main` package) exporting shared utilities for HTTP services.

**Packages:**

- `cookie` — Secure cookie encode/decode via `gorilla/securecookie`. Singleton `SecureCookie` initialized once via `sync.Once`. Call `InitSecureCookie(hashKey, blockKey)` at startup with stable keys; omitting it auto-generates random keys (useless across restarts).

- `session` — `Cookie` struct representing a session (ID, expiry, authenticated flag). Used by `middleware` to validate auth.

- `middleware` — Standard `net/http` middleware wrappers: cookie-based auth (`GetAuthMiddlewareForCookie`), API key auth (`GetAuthMiddlewareForAPIKey`), structured request logging, and panic recovery. HTMX requests are handled separately in the auth middleware (sets `HX-Refresh` instead of redirect).

- `pubsub` — (deprecated, no known consumers) `UniversalPubSub` wraps either `LocalPubSub` (in-memory channels, single-process) or `RedisPubSub`. Selected by `NewUniversalPubSub(opts)` based on whether `opts.Redis` is set. Channel names are prefixed with `opts.Prefix` + `:` when non-empty.

- `metrics` — Prometheus-compatible metrics (VictoriaMetrics/metrics): `/metrics` handler with Go/process metrics, route-pattern-labelled HTTP middleware, counters/gauges/histograms, optional separate listener.

- `web` — HTTP response helpers (`Write`, `WriteString`, `CheckError`). `CheckError` logs errors via `slog` and writes `http.Error`.

- `environment` — Loads `.env` then `.override.env` then optional extra files via `godotenv.Overload`.

- `log` — Structured logging helpers on `log/slog`: `InitLogger` with `FormatText` (tint-colored) or `FormatJSON`, `LevelFromString`, `ParseFormat`. No tdewolff dependency.
- `minify` — JS minification (`MinifyJS`, `GetMinifier`) via `tdewolff/minify`.
- `utils` — Miscellaneous: `WrapErrors`, `RandString`, generics (`Filter`), IP utilities, version parsing. Deprecated shims (one release): `InitLogger`/`LoggerConfig`/`LevelFromString`/`ParseFormat` (use `log`), `MinifyJS`/`GetMinifier` (use `minify`), plus `RunCmd`/`RunCmdInPath`, `Contains`, `PointerTo`.

- `oidcauth` — OIDC sign-in + server-side sessions + MCP bearer auth as one package, generic over the app identity type (`Config[T].Authorize` maps verified `IdentityInfo` claims to `T`; `IdentityInfoAuthorize` covers `T=IdentityInfo`). Modelled on basket's `internal/auth`. Apps append `oidcauth.Migration` (sessions + oidc_transactions tables) to their store migrations and pass a shared `*sqlx.DB`. Cookie names, app name, success path, and an optional discovery HTTP client are config; proton-specific behaviour (rotating renewal tokens, sign-in redirects) stays in proton-mcp.

- `sqlitestore` — SQLite scaffold modelled on basket's `internal/store`: `Open(ctx, path, migrations)` with WAL/foreign-keys/busy-timeout pragmas, single connection, ordered `PRAGMA user_version` migrations (`Migrate`, `ErrSchemaVersion` beyond the list), `Read`/`Write` tx helpers; `Tx.Inner()` exposes the `*sqlx.Tx` for app queries, `DB()` the `*sqlx.DB`.

- `backup` — Periodic `VACUUM INTO` SQLite backups (`Run`, one-shot `Copy`) against an `Execer` interface (`*sqlx.DB`/`*sql.DB`), configurable file prefix (`WithFilePrefix`) and retention pruning.

- `mockidp` — Test-only mock OIDC provider (mockoidc) on a loopback port: `Start` for Go tests (underlying provider via `Provider()`), `Run(ctx, out)` writes the `Config` JSON line and blocks, for external e2e harnesses via a small local `main`.

## Linting Rules

The linter (`golangci-lint`, config in `.golangci.yaml`) enforces several non-obvious rules:

- **No `testify`** — use `cmp` or standard library assertions in tests.
- **No `math/rand`** — use `math/rand/v2`.
- **No `io/ioutil`** — use `os.ReadFile`, `io.NopCloser`, etc.
- **No `log.Print*`/`log.Fatal*`/`os.Exit`/`time.Sleep`/`context.TODO`** — use `slog`, context with timeout, etc.
- **`slog` style**: `attr-only` (no kv mixed args), camelCase key names, capitalized messages.
- **Max line length**: 100 characters (golines).
- **Import order**: standard → default → project prefix → blank → dot → alias → localmodule.
- **Struct tag order**: `description`, `json`, `toml`, `yaml`/`yml`, `label`, `file`, `kv`, `export`.
- **`exhaustive`**: all switch/map enumerations must have a default case.
