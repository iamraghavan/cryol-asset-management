# Cryol Asset Management handover

## Start here

Continue inside this repository. Cryol is a self-hosted enterprise IT asset application for **one organization per installation**, with a SaaS-style web console and a perpetual-license product model. Keep the existing React/Vite, Go, and PostgreSQL architecture. The service and agents must work on a private LAN using locally bundled assets and without cloud dependencies. Client software entitlements may have renewals. Cryol itself has no subscription billing.

The current user priority is **simpler endpoint setup**, especially on Windows. The last discussion proposed a native installer that discovers or receives the appropriate Cryol server address, enrolls the endpoint with an expiring one-use credential, starts a background service, and gives separate feedback for connection and first inventory. A console ZIP now includes an administrator-approved launcher that registers a native Windows Service. Signed single-file installer packaging and native Windows verification remain open.

The repository was on `main` with no commits or configured remote at handover. The application source, docs, and lockfiles are existing untracked work. Keep them intact and check `git status --short --branch` before changes. Generated frontend builds, dependencies, test output and temporary binaries are reproducible locally and need separate handling when transferring the workspace. The current environment's read-only `.git` policy may also affect commits.

## Current implementation

| Area | Working | Open |
| --- | --- | --- |
| Identity and settings | Interactive administrator bootstrap, Argon2id, PostgreSQL-backed sessions, permissions, CSRF checks, organization profile and first-run dashboard navigation | Managed users, recovery and password reset, full role set, department and branch scopes |
| Manual asset registry | Numbered assets, server-side search/sort/filter/pagination, view and edit drawers, archive/restore, audit history, revision checks, selected-row CSV | Classification and tags, additional purchase/warranty fields, attachments, full reports and custody |
| Dashboard | PostgreSQL total, active and archived manual-asset counts | Device, software and agent health metrics and trends |
| Endpoint agents | Single-use enrollment, per-agent credentials, heartbeat-based online status, revocation, versioned idempotent snapshots, explicit asset linking | Credential rotation, scan control, service installation, native platform acceptance |
| Inventory console | Agent list, latest hardware and network view, searchable and paginated software from the last complete scan | Normalized catalog, hardware/software changes, cross-device reporting, endpoint classification and license reconciliation |
| Agent launcher | Authenticated Windows and Linux x64 ZIP downloads with server address prefilled, optional internal CA file, auto-enrollment prompt and first scan, five-minute scheduled scans while open | Native Windows installer, DEB/RPM, persistent service, verified restart/uninstall and guided server reachability checks |
| Other operations | Roadmap and architecture | Organization directory, assignments, transfers, maintenance, procurement, backup/restore workflows and offline release packaging |

Cryol has an operational foundation and selected verified workflows. The wider enterprise release and installer acceptance remain open.

## Code map

| Path | Responsibility |
| --- | --- |
| `apps/web/src/main.tsx` | Client routes. Go also registers direct-entry console URLs in `internal/app/http.go`. |
| `apps/web/src/workspace.tsx` and `components/console-shell.tsx` | Sign-in, app shell, permissions, navigation, organization settings, dashboard and search |
| `apps/web/src/asset-registry.tsx` | Registry list, filters, selection, CSV and asset record drawers |
| `apps/web/src/agent-console.tsx` | Endpoint list, setup downloads, tokens, inventory and asset linking |
| `apps/server/internal/app/identity.go`, `assets.go`, `agents.go`, `agent_setup.go` | HTTP handlers, RBAC and setup bundles |
| `apps/server/internal/auth` | Password hashing, bootstrap and server-side sessions |
| `apps/server/internal/inventory/contract.go` | Versioned inventory schema and validation shared by server and agent |
| `apps/server/cmd/cryol-agent` | Enrollment, scan, scheduled foreground loop, queue, platform-specific collectors |
| `apps/server/cmd/cryol-server`, `cmd/cryol-cli` | Server entry point, schema migration and administrator bootstrap |
| `apps/server/migrations` | Embeds Goose migrations 1–5. Runtime requires schema version 5. |

Browser API routes sit under `/api/v1`. The agent uses outbound `/api/agent/v1/enroll`, `heartbeat` and `inventory` with verified HTTPS and per-agent bearer credentials after enrollment. Browser setup routes are `GET /api/v1/agents/setup` and `GET /api/v1/agents/setup/{platform}`. Token issuance returns the secret once. Database records hold SHA-256 digests. A snapshot run is keyed by agent UUID and run UUID. A partial software scan retains the previous complete view.

## Run locally

The verified environment used Go 1.26.4, Node.js 22.15.1, Corepack/pnpm and PostgreSQL 17.9 on macOS x86-64. Use a dedicated PostgreSQL database. Use fresh disposable databases for integration and browser tests. The CLI only applies forward migrations. Inspect and back up any existing database before applying a new version.

From the repository root, copy `.env.example` to a private `.env`, fill in the provisioned database URL, then run:

```sh
corepack pnpm install --frozen-lockfile
corepack pnpm build
set -a
. ./.env
set +a
export CRYOL_WEB_DIR="$PWD/apps/web/dist"
cd apps/server
go run ./cmd/cryol-cli migrate
go run ./cmd/cryol-cli schema-check
# Only on a fresh installation. The command prompts for the password.
go run ./cmd/cryol-cli bootstrap-admin admin
go run ./cmd/cryol-server
```

The default URL is `http://127.0.0.1:8080`. Main routes are `/console/dashboard`, `/console/assets`, `/console/agents`, `/console/settings/organization` and `/system`. Readiness is `GET /api/v1/system/readiness`. For frontend hot reload, run `corepack pnpm dev` from the repository root while the Go API is running. Vite uses port 5173 and proxies `/api` to Go.

On the original workstation the development database and binary artifacts were under `/private/tmp/cryol-pg-build` and `/private/tmp/cryol-tools`. The last verified server ran on port 8080 and PostgreSQL on loopback port 55432. Those artifacts and processes may be gone on a new machine or session. The original administrator password was generated and stored in a local private file under `/private/tmp/cryol-tools/local-login.txt`. Provision separate credentials for fresh environments. The requested six-digit replacement was never applied. Bootstrap requires a passphrase of at least 15 characters. There is no implemented password-reset command.

## Agent setup and network behavior

- To enable the existing downloads, point `CRYOL_AGENT_DOWNLOAD_DIR` at an admin-managed directory with `cryol-agent.exe` for Windows x64 and `cryol-agent` for Linux x64. From `apps/server`, cross-build with `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /absolute/bundle-dir/cryol-agent.exe ./cmd/cryol-agent` and `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /absolute/bundle-dir/cryol-agent ./cmd/cryol-agent`.
- Open **Endpoint agents → New enrollment** through the URL the target endpoint can reach. Download and extract the matching bundle, generate a one-use token, and launch **Start Cryol Agent.cmd** on Windows or `sh start-cryol-agent.sh` on Linux. Paste the token. Launchers use `setup`, enroll when the selected state directory has no identity, then scan and enter the foreground loop. They reuse the identity for the same origin and scan about every five minutes while running. Direct CLI `run` defaults to daily scans and a five-minute heartbeat.
- For a separate computer or VM, use the server's reachable HTTPS hostname or IP. Set the server bind address and `CRYOL_TLS_CERT` / `CRYOL_TLS_KEY`. The certificate must match the hostname/IP and be trusted by the endpoint. A private CA can be installed in the endpoint trust store or placed beside the launcher as `cryol-ca.pem`. Only the public CA certificate belongs on the endpoint.
- `CRYOL_AGENT_LOCAL_HTTP=1` allows explicitly enabled loopback testing. The server checks both loopback peer and host, and the agent only dials loopback with `-local-http`. A Windows guest's localhost refers to the guest. The macOS server can host the console, while native inventory collection currently targets Windows and Linux.
- Setup ZIPs contain the launcher and binary. Tokens are entered separately and expire after 15 minutes. Signed installers, unattended provisioning and peer-to-peer scans are pending. The Windows launcher registers the CryolAgent service. The Linux launcher remains foreground.
- Linux coverage includes supported system files, DMI where readable, network adapters and dpkg/rpm packages. Windows coverage includes system metadata and machine-wide 32-bit/64-bit uninstall registry views. Neither collector executes discovered software or runs commands from the server. Document collection gaps and preserve partial-scan signaling.

## Verification

```sh
corepack pnpm build
corepack pnpm lint
corepack pnpm test
(cd apps/server && go vet ./... && go test ./...)
```

To run live database tests, set `CRYOL_TEST_DATABASE_URL` and `CRYOL_IDENTITY_TEST_DATABASE_URL` to **two separate fresh disposable databases** and execute `go test -count=1 -v ./...` from `apps/server`. The identity suite creates data and changes grants. To run Playwright, point `CRYOL_E2E_URL` at a disposable server and set `CRYOL_E2E_USERNAME` and `CRYOL_E2E_PASSWORD`. Install Chromium through Playwright or supply a compatible binary via `CRYOL_E2E_BROWSER`. Run `corepack pnpm --filter @cryol/web exec playwright test`. Set `CRYOL_E2E_SETUP=1` for the generated Windows ZIP test and configure setup downloads and loopback HTTP on that disposable test server.

Executed checks include frontend build/lint/Vitest, Go vet and PostgreSQL integration, browser asset CRUD and download/token workflows, plus Linux and Windows x64 cross-builds. Last live verification inspected both generated setup ZIPs and confirmed their embedded local server address. Existing organization and asset fields matched before and after migration 5. Native endpoint enrollment, scans, service startup and install/uninstall have yet to be verified on the target operating systems. Hosted CI and production load results remain unverified.

## Recommended continuation

1. **Simplify Windows setup.** Build a native, visible installer or bootstrapper with a server URL and one-use enrollment process. Keep secrets out of command-line arguments, ordinary logs and installer binaries. Preserve the manual ZIP as a fallback. Surface reachable server, enrollment and first-scan states in the console. Verify on a Windows test device or VM through LAN HTTPS.
2. Add a supported Windows Service and Linux systemd unit with least-privilege state storage. Test restart, interrupted enrollment, revocation, reinstall and uninstall on target systems. The current state uses an exclusive lock that may need documented cleanup after a crash.
3. Add asset and device classifications and tags, then the normalized software catalog, version/change history and entitlement reconciliation. Keep device reconciliation administrator-controlled. A hostname, IP or MAC alone is insufficient to merge endpoints.
4. Finish users, roles and organizational scopes before exposing sensitive operations to non-admins. Follow with directory entities, custody, reports, notifications and tested backup/restore.
5. Make new migrations additive and forward-only. Preserve older migrations and verify data before and after deployment. Keep user-facing navigation limited to working features. Retain read-only agent collection and the current transport/authentication protections.

## References

- [Development setup](README.md)
- [Current implementation and verification](docs/development/implementation-status.md)
- [Roadmap](docs/development/roadmap.md)
- [Application audit](docs/audit/application-audit.md)
- [Endpoint operating guide](docs/operations/endpoint-agents.md)
- [Endpoint API](docs/api/endpoint-inventory.md)
- [Browser OpenAPI contract](docs/api/openapi.yaml)
- [Schema](docs/database/schema.md)
