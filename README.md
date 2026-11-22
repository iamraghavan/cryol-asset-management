# Cryol Asset Management

Cryol is a single-organization, self-hosted asset management application in development. The current build includes local sign-in, organization profile management and a PostgreSQL system-status screen.

## Local development

Use Go 1.26, Node.js 22.12 or newer, Corepack and a dedicated PostgreSQL 17 development database. Install frontend dependencies with `corepack pnpm install --frozen-lockfile`. Copy `.env.example` to `.env` and configure `CRYOL_DATABASE_URL` with the provisioned database credentials. Use TLS when connecting across an untrusted network.

From the repository root, load the environment and start the API.

```sh
set -a
. ./.env
set +a
cd apps/server
go run ./cmd/cryol-cli migrate
go run ./cmd/cryol-cli schema-check
go run ./cmd/cryol-cli bootstrap-admin admin
go run ./cmd/cryol-server
```

In a separate terminal, run `corepack pnpm --filter @cryol/web dev` and open http://localhost:5173. The development server proxies API requests to the Go listener. Set `CRYOL_DEV_API_ORIGIN` if the Go listener uses a different address.

Run frontend checks with `corepack pnpm --filter @cryol/web lint`, `corepack pnpm --filter @cryol/web test` and `corepack pnpm --filter @cryol/web build`. Run `go test ./...`, `go vet ./...` and `go build ./...` from `apps/server`. Set `CRYOL_TEST_DATABASE_URL` to a fresh disposable PostgreSQL database to include the integration test.

To serve the compiled frontend from Go, build the frontend and set `CRYOL_WEB_DIR` to the absolute path of `apps/web/dist` when starting the server.

- [Architecture](docs/architecture/system-overview.md)
- [Migration procedure](docs/database/migrations.md)
- [OpenAPI contract](docs/api/openapi.yaml)
- [Milestone roadmap](docs/development/roadmap.md)
- [Implementation status](docs/development/implementation-status.md)

The bootstrap command prompts twice for a password of at least 15 characters and works once per installation. Open the root page to sign in. Use the system-status link to check readiness.

The default listener accepts HTTP on a loopback IP for development. For LAN deployment set `CRYOL_HTTP_ADDR` and provide the file paths for `CRYOL_TLS_CERT` and `CRYOL_TLS_KEY`. The service uses HTTPS directly.

For identity integration tests, set `CRYOL_IDENTITY_TEST_DATABASE_URL` to a second disposable empty database. The test creates an administrator and modifies role grants and session timestamps in that database.

## Asset registry browser test

Build the frontend, migrate a disposable PostgreSQL database, bootstrap its administrator and start Cryol with the built frontend. Set `CRYOL_E2E_URL`, `CRYOL_E2E_USERNAME` and `CRYOL_E2E_PASSWORD` in the test process environment. The test creates an organization profile if required and writes an asset. Use a test installation. Install Chromium with `corepack pnpm --filter @cryol/web exec playwright install chromium`, or set `CRYOL_E2E_BROWSER` to a compatible Chromium executable. Run `corepack pnpm --filter @cryol/web exec playwright test`.
