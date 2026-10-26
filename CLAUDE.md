# CLAUDE.md

A WebDAV reverse proxy between Zotero and pCloud, written in Go with only the standard library. See `README.md` for what it does and how to configure it.

## Checks

Run these before every commit; each commit must pass on its own:

```sh
gofmt -l .        # must print nothing
go vet ./...
go test -race ./...
```

The module needs Go 1.27 or later (`go.mod`); the toolchain downloads it automatically if an older Go is installed. Tests use `httptest.NewTestServer`, which older Go versions lack.

## Layout

| File | Responsibility |
| --- | --- |
| `main.go` | Server construction, timeouts, graceful shutdown, wiring, `healthcheck` subcommand |
| `config.go` | Environment variables → `config`, with validation |
| `health.go` | `/healthz` handler and the client the `healthcheck` subcommand uses |
| `auth.go` | Basic Auth check of Zotero's proxy credentials |
| `proxy.go` | Reverse proxy: path validation, root scoping, URL rewriting |
| `logging.go` | Request IDs and the single `logRequest` helper |
| `Dockerfile`, `compose.yaml` | Container image and an example deployment |

Handler chain (`newHandler`): `withRequestID` → `withHealthCheck` → `requireAuth` → `webdavProxy` → pCloud.

## Conventions

- **Commits:** small, self-contained commits, each passing the checks above. The message body explains *why*, not just what. Separate refactors from behaviour changes.
- **Comments:** sparse; a short comment above a function or on a non-obvious line, explaining intent or a constraint rather than restating the code.
- **Tests:** table- or `t.Run`-style subtests named as sentences. Tests that need an upstream use a real loopback server (`httptest.NewTestServer` + `Start()`), because the proxy uses its own `http.Transport`. When fixing a bug, check that the new test fails without the fix.
- **Dependencies:** standard library only unless there's a strong reason.
- **Logging:** always go through `logRequest`. Never log headers, credentials or request bodies. Paths are logged with `%q` because they're client-controlled.
- **Config:** everything comes from environment variables, read through `loadConfig(getenv)` so tests can pass a map instead of touching the process environment. Fail at startup on anything missing or invalid, naming the variable.

## Decisions already made

Don't undo these without discussing it first:

- **No `ReadTimeout`/`WriteTimeout` on the server.** They bound the whole body transfer and would cut off large attachment uploads and downloads. `ReadHeaderTimeout` and `IdleTimeout` guard against slowloris; bodies are only read after authentication.
- **The listener is created separately from the server** (`net.Listen` in `main`, `run(ctx, srv, ln)`). This lets bind errors fail fast before "Begin listening" is logged, and lets tests listen on port 0.
- **`PCLOUD_ROOT` is required and can't be `/`.** Confining Zotero to one folder is a core reason the proxy exists.
- **Paths are validated explicitly in `validatePath`, not left to `http.ServeMux`.** The mux only cleans paths by redirecting and leaves `%2F` alone, and `httputil.ReverseProxy` does no cleaning. Reject dot segments (decoded), `%2F`/`%5C`, backslashes and NUL with 400 before anything reaches pCloud.
- **URL rewriting is two-way.** `Destination` goes proxy → pCloud; `Location` and multistatus `href`s go pCloud → proxy, by stripping the root prefix. Zotero checks that PROPFIND hrefs sit under the URL it requested. The client's `Accept-Encoding` is dropped for everything except GET/HEAD so the transport decodes gzip and bodies can be rewritten.
- **Credentials are compared as SHA-256 digests in constant time**, so timing doesn't leak their length.
- **`PCLOUD_REGION` (`us`/`eu`) chooses the endpoint.** `PCLOUD_URL` is only an override for test servers, holds only a scheme and host, and can't be combined with `PCLOUD_REGION`.
- **The upstream transport is cloned from `http.DefaultTransport`** for its dial/TLS timeouts and `HTTPS_PROXY` support; don't go back to a bare `http.Transport`.
- **`LISTEN_ADDR` defaults to `127.0.0.1:8080`.** The image overrides it to `0.0.0.0:8080`. TLS belongs in a reverse proxy such as Caddy, not in this binary.
- **`/healthz` is answered before authentication and never reaches pCloud**, so a pCloud outage doesn't get a working container restarted. It's matched exactly, not through `http.ServeMux`, and isn't logged.
- **The image is distroless `static:nonroot`, with base images pinned by digest.** It has no shell or curl, so `HEALTHCHECK` runs the binary's own `healthcheck` subcommand. `.dockerignore` is an allow-list so `.env` can't enter the build context.
- **Stop timeouts exceed the 30s shutdown window** (`stop_grace_period: 40s`); Docker's 10s default would cut off uploads.
- **Configuration in containers comes from an env file** (`.env`, gitignored), not Docker secrets.

## Known gaps / backlog

Parked deliberately, roughly in priority order:

1. **Method allow-list:** optionally restrict to the methods Zotero uses (OPTIONS, PROPFIND, GET, PUT, DELETE, MKCOL).
2. **Not yet verified against real pCloud and Zotero:** run "Verify Server", a sync, and orphaned-file cleanup, and confirm the href rewriting with real responses. Also check whether the 30s `ResponseHeaderTimeout` is enough for a large `PROPFIND Depth: 1`.
3. **Image publishing:** a GitHub Actions workflow to build multi-arch images and push them to GHCR, with Dependabot bumping the pinned base image digests. Held off for now; images are built locally from `compose.yaml`.
