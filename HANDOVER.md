# Cryol Asset Management handover

Use this file to set up Cryol after cloning it on another laptop and to brief Codex on that machine. Cryol is a self-hosted IT asset management application for one organization. It uses React/Vite, Go, PostgreSQL, and separately installed endpoint agents.

## 1. Clone or update

The repository is private. Use a GitHub account with access to `iamraghavan/cryol-asset-management`.

```sh
git clone https://github.com/iamraghavan/cryol-asset-management.git
cd cryol-asset-management
```

If already cloned, check for local changes before pulling.

```sh
git status --short --branch
git switch main
git pull --ff-only origin main
```

On the second laptop, open this folder in Codex and ask it to read `HANDOVER.md`, check `git status`, and verify local prerequisites before making changes.

## 2. Prepare PostgreSQL

Install and start PostgreSQL 17 with the command-line tools on your PATH. Install Go 1.26.4 and Node.js 22.12 or newer with Corepack. The default PostgreSQL port is 5432. Confirm your installation's actual port and administrator name.

```sh
go version
node --version
corepack --version
psql --version
pg_isready -h 127.0.0.1 -p 5432
psql -h 127.0.0.1 -p 5432 -U postgres -d postgres
```

In `psql`, create a dedicated database and login once. Use the installed PostgreSQL administrator instead of `postgres` if your setup uses a different role. On Linux with peer authentication, `sudo -u postgres psql -d postgres` may be the administrative entry point.

```sql
CREATE ROLE cryol LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
\password cryol
CREATE DATABASE cryol OWNER cryol;
\q
```

Enter a new PostgreSQL password at the prompt. Test it before configuring the application.

```sh
psql -h 127.0.0.1 -p 5432 -U cryol -d cryol -W -c "select current_user, current_database();"
```

If the role or database already exists, verify it belongs to this installation before using it.

## 3. Configure the database and private environment

Choose one path. Both start from a repository root with Go available and use the terminal setup wizard.

### Fresh installation

```sh
cd apps/server
go run ./cmd/cryol-cli setup --write-env ../../.env
cd ../..
```

Enter the PostgreSQL connection URL when requested. Replace the password placeholder with the password for the PostgreSQL role. Use the actual database port. Percent-encode reserved URL characters such as `@` in the password.

```dotenv
CRYOL_DATABASE_URL='postgres://cryol:REPLACE_WITH_LOCAL_DB_PASSWORD@127.0.0.1:5432/cryol?sslmode=disable'
CRYOL_HTTP_ADDR='127.0.0.1:8080'
```

The wizard verifies the target, applies Goose migrations, and prompts for the first application administrator. Use at least 15 characters for the Cryol admin password. This password is separate from the PostgreSQL role password. On Windows, use PowerShell or Windows Terminal for the wizard's password prompts.

### Move an existing installation

Create a custom-format backup on the original machine with the source database host, port, role, and name. Stop writes first if the backup must include all activity through cutover.

```sh
pg_dump -h OLD_HOST -p OLD_PORT -U OLD_ROLE -d OLD_DATABASE -W -Fc -f cryol-backup.dump
```

Transfer the backup privately. On the second laptop, use the dedicated empty destination database created above. Ensure `pg_restore` is on PATH and compatible with the source backup.

```sh
cd apps/server
go run ./cmd/cryol-cli setup --write-env ../../.env --restore "/path/to/cryol-backup.dump"
cd ../..
```

The wizard asks for confirmation, restores into the empty database in one transaction, checks the Cryol schema, and guides any pending migrations. Sign in with an existing Cryol account from the backup. If the second laptop changes the HTTPS origin, review existing agents' configured server addresses and certificate trust before cutover.

### Environment reference

Git tracks `.env.example` and `.env.docker.example`. The local `.env` and any `*.local.txt` environment snapshots are ignored. A Git clone brings over source, while the database, passwords, private keys, and local agent artifacts must be configured or transferred separately.

| Variable | Local setup |
| --- | --- |
| `CRYOL_DATABASE_URL` | Required. PostgreSQL URL for the dedicated role and database. Use `sslmode=disable` only for a database on the same laptop. |
| `CRYOL_HTTP_ADDR` | `127.0.0.1:8080` for local HTTP. LAN access needs a reachable address and HTTPS. |
| `CRYOL_WEB_DIR` | Set to the absolute path to `apps/web/dist` to serve the compiled frontend. |
| `CRYOL_TLS_CERT`, `CRYOL_TLS_KEY` | Leave empty for loopback HTTP. Provide both for HTTPS. The certificate must cover the address clients use. |
| `CRYOL_AGENT_SETUP_ORIGIN`, `CRYOL_AGENT_SETUP_CA` | Optional endpoint-facing HTTPS origin and public CA PEM path for agent downloads. |
| `CRYOL_AGENT_DOWNLOAD_DIR`, `CRYOL_WINDOWS_AGENT_INSTALLER` | Optional paths to locally staged agent binaries and Windows installer. |
| `CRYOL_AGENT_LOCAL_HTTP` | Default `0`. Set `1` only for same-machine loopback agent tests. |

When `.env` already exists, load it and run `go run ./cmd/cryol-cli setup` without `--write-env`. The wizard preserves existing files. Read [endpoint agent operations](docs/operations/endpoint-agents.md) for installer options.

## 4. Build and run

From the repository root in macOS/Linux or Git Bash:

```sh
corepack pnpm install --frozen-lockfile
corepack pnpm build
set -a
. ./.env
set +a
export CRYOL_WEB_DIR="$PWD/apps/web/dist"
cd apps/server
go run ./cmd/cryol-cli schema-check
go run ./cmd/cryol-server
```

For Windows PowerShell, use these equivalent commands after the wizard has written its two-key `.env`:

```powershell
corepack pnpm install --frozen-lockfile
corepack pnpm build
foreach ($key in 'CRYOL_DATABASE_URL', 'CRYOL_HTTP_ADDR') {
    $line = Get-Content .env | Where-Object { $_ -like "$key=*" } | Select-Object -First 1
    if ($line) {
        [Environment]::SetEnvironmentVariable($key, ($line -split '=', 2)[1].Trim("'"), 'Process')
    }
}
$env:CRYOL_WEB_DIR = (Resolve-Path apps/web/dist).Path
Set-Location apps/server
go run ./cmd/cryol-cli schema-check
go run ./cmd/cryol-server
```

Open **http://127.0.0.1:8080**. Sign in and finish the organization profile for a fresh installation. In another terminal, check readiness:

```sh
curl http://127.0.0.1:8080/api/v1/system/readiness
```

Expected response is `{"status":"ready"}`. For frontend hot reload, keep Go running and use `corepack pnpm dev` from another terminal at the repository root. Open **http://localhost:5173**. Docker is also available in [SETUP-SECOND-LAPTOP.txt](SETUP-SECOND-LAPTOP.txt).

## 5. Migration and troubleshooting notes for Codex

- Goose migrations are embedded from `apps/server/migrations`. Server startup checks schema compatibility. For an existing Cryol database, take and verify a backup before applying pending migrations. With `.env` loaded and the shell in `apps/server`, use `go run ./cmd/cryol-cli migrate` followed by `go run ./cmd/cryol-cli schema-check`.
- If startup cannot connect, check PostgreSQL service status and confirm the app role can sign in using the same host, port, and database as `.env`.
- If port 8080 is already in use, set an available loopback port in `CRYOL_HTTP_ADDR`. For Vite development, point `CRYOL_DEV_API_ORIGIN` to the changed port.
- Before editing, check Git status and preserve existing changes. Use disposable databases for integration and browser tests. Keep credentials, real backups, and private keys out of commits.
- Run `go test ./...` and `go vet ./...` in `apps/server`. Run `corepack pnpm lint`, `corepack pnpm test`, and `corepack pnpm build` at the repository root.

## 6. Code map

| Path | Responsibility |
| --- | --- |
| `apps/server/cmd/cryol-cli` | Database verification, migrations, setup wizard, restore, and admin bootstrap |
| `apps/server/cmd/cryol-server` | HTTP server, TLS listener, and startup readiness |
| `apps/server/cmd/cryol-agent` | Platform collectors, enrollment, and scheduled authenticated uploads |
| `apps/server/internal/app` | Session-protected APIs, asset registry, agent management, and setup downloads |
| `apps/web/src/workspace.tsx` | Sign-in, dashboard, and organization settings |
| `apps/web/src/asset-registry.tsx`, `agent-console.tsx` | Asset and endpoint views |
| `installer/windows/agent` | Windows endpoint setup and recovery installer |

See [implementation status](docs/development/implementation-status.md) for delivered and pending modules. The Windows agent setup is separate from setting up the Cryol server.
