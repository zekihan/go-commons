# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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

- `pubsub` — `UniversalPubSub` wraps either `LocalPubSub` (in-memory channels, single-process) or `RedisPubSub`. Selected by `NewUniversalPubSub(opts)` based on whether `opts.Redis` is set. Channel names are prefixed with `opts.Prefix` + `:` when non-empty.

- `web` — HTTP response helpers (`Write`, `WriteString`, `CheckError`). `CheckError` logs errors via `slog` and writes `http.Error`.

- `environment` — Loads `.env` then `.override.env` then optional extra files via `godotenv.Overload`.

- `utils` — Miscellaneous: `InitLogger` (tint-colored slog), `WrapErrors`, `RandString`, generics (`Contains`, `Filter`, `PointerTo`), IP utilities, HTML minification, version parsing, command execution helpers.

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
