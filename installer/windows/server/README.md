# Cryol Windows server setup

This launcher installs and runs the Cryol server on a Windows 10/11 x64 laptop.
The endpoint agent installer is separate.

## Download and run

1. Sign into GitHub with access to iamraghavan/cryol-asset-management.
2. Open the repository's **Actions > Windows server setup** and select a successful
   run for the commit you want. Download **CryolSetup-win-x64** under Artifacts.
3. Extract the complete ZIP. Keep CryolSetup.exe, CryolSetup.dll, and the other
   runtime files together. Double-click **CryolSetup.exe** and accept elevation.
4. Enter an empty destination folder or the path to an existing Cryol checkout.
5. Enter and confirm your desired local PostgreSQL password. Use your requested
   local password here; credentials are not embedded in the download or committed.
6. If an existing PostgreSQL administrator password differs, enter it when asked.
   If forgotten, type RESET when prompted to reset the matched local service's
   postgres login to your chosen password. Authentication rules are restored in
   a finally block, including if the reset fails.
7. For a fresh database, enter a Cryol administrator username and a separate
   application password of at least 15 characters, twice.
8. Setup opens http://127.0.0.1:8080 only after readiness returns status ready.

An internet connection, Windows administrator rights, GitHub access to the private
repository, and Microsoft App Installer (winget) are required. Git Credential
Manager may open a GitHub sign-in window during clone. This cannot bypass private
repository authentication. No .NET installation is required for the downloaded
self-contained EXE.

## Run the PowerShell source

In an Administrator PowerShell terminal:

~~~powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\installer\windows\server\Setup-Cryol.ps1
~~~

A downloaded EXE accepts these optional arguments:

~~~powershell
.\CryolSetup.exe -ProjectPath 'F:\Projects\Cryol' -DatabasePort 5432 -AppPort 8080
~~~

Use -ResetPostgresPassword to authorize local administrator password recovery
without the RESET confirmation, or -NoBrowser to skip browser launch and the final
Enter prompt. Password entry and first application administrator creation still
require an interactive terminal.

## What setup does

- Checks Git, Node.js >=22.12, Go >=1.26.4, and Corepack. Installs missing tools
  through winget/npm and upgrades outdated Go/Node installations.
- Finds an installed PostgreSQL Windows service by its configured port, including
  PostgreSQL 17 and 18. Starts it and verifies readiness. If none exists, installs
  PostgreSQL 17 server and command-line tools using the EDB unattended installer
  with a temporary, restricted option file that is deleted afterward.
- Creates the unprivileged cryol role and cryol database. Existing role credentials
  and database ownership are verified. It does not drop databases, overwrite an
  existing application's password, or adopt a database owned by another role.
- Writes a UTF-8 .env for a new installation; preserves and validates an existing
  .env. Only the dedicated local cryol database with sslmode=disable is supported.
- Before migrating an existing Cryol schema, writes a custom-format pg_dump backup
  to %LOCALAPPDATA%\Cryol\backups and checks its archive table of contents.
  Backups contain private data and must be stored securely.
- Builds the frontend, server, CLI, and all five desktop agent binaries. Configures
  CRYOL_AGENT_DOWNLOAD_DIR and CRYOL_AGENT_LOCAL_HTTP=1 so Agent downloads lists ZIPs
  when the console is opened on this laptop. Existing database configuration and
  custom agent directories are preserved. Custom directories must contain their
  own Windows agent binary.
- Runs migrate and schema-check directly, without
  invoking cryol-cli setup. Bootstraps an application administrator only if no users
  exist. Existing application accounts are preserved.
- Runs the server as a hidden background process and checks HTTP readiness.
  A failed readiness check stops only the process launched by this invocation.

The existing checkout is used as-is: setup does not switch branches, pull, merge,
or discard local changes. Empty destinations are cloned from the repository's
default branch. The Git trust exception is scoped to individual commands, not
added to global Git configuration.

## Restart and stop

Compiled binaries, logs, and the restart script are in PROJECT\.cryol-local.
The database URL and HTTP address are in PROJECT\.env, which is ignored by Git.
Both paths are restricted to the current Windows user, Administrators, and SYSTEM.

To start again after a reboot, run in PowerShell:

~~~powershell
powershell -NoProfile -ExecutionPolicy Bypass -File 'F:\Projects\Cryol\.cryol-local\Start-Cryol.ps1'
~~~

Replace the example path with your project folder. Keep that terminal open.
Alternatively rerun the setup launcher. A running server from the same checkout
is stopped before backup and migration, then restarted after a successful build.
This applies repaired environment settings to an already-running installation.
Other processes occupying the selected port are left running.

The setup completion message prints the server PID and Stop-Process command.
Startup logs are .cryol-local\server.out.log and .cryol-local\server.err.log.
The launcher does not register a startup task or Windows service.

This local launcher binds to loopback. Other computers cannot access 127.0.0.1 on
this laptop; LAN deployment requires HTTPS and a reachable address as described in
HANDOVER.md. Existing TLS configurations are preserved and cause this local
launcher to stop with guidance rather than downgrading the deployment.

## Build the EXE and DLL yourself

With the .NET 10 SDK installed, from the repository root:

~~~powershell
dotnet publish installer/windows/server/CryolSetup.csproj -c Release -r win-x64 --self-contained true -o dist/CryolSetup-win-x64
~~~

Distribute the entire output directory. CryolSetup.dll is the managed launcher
assembly used by CryolSetup.exe; it is not a Windows service or a native DLL
intended for regsvr32 or rundll32. The executable is not code-signed.

## Checks and limitations

Test-Setup.ps1 checks Windows PowerShell parsing, SQL quoting, URL encoding,
environment-file round trips and preservation, and byte-for-byte restoration of
authentication configuration after simulated failure. Test-Database.ps1 provisions
a disposable PostgreSQL database in CI, checks reruns and bad credentials, runs
migrations twice, checks the schema, and verifies real server readiness.

CI compiles and packages the Windows launcher and runs the pure setup checks in
Windows PowerShell 5.1. Interactive GitHub login, UAC, winget/EDB installation, and
administrator password prompts require a manual test on a fresh laptop. A machine
with winget disabled, a managed installation policy, an unsupported PostgreSQL
service layout, an occupied port, or mismatched existing credentials stops with an
error rather than reporting success.

Password recovery needs write access to the matched service's pg_hba.conf.
Do not interrupt the process while recovery is active. A uniquely named original
configuration backup remains beside pg_hba.conf for recovery if Windows or the
process is forcibly terminated.

## Repair missing agent downloads on an existing laptop

Download the latest CryolSetup-win-x64 artifact and rerun it against the same
project folder. Use the existing PostgreSQL port (for example -DatabasePort 5000)
and application port. Existing database contents and application accounts remain.

Setup now stages Windows x64, Linux x64/ARM64, and macOS Intel/Apple silicon agent
binaries in .cryol-local\agent-binaries, enables same-machine downloads in .env,
and restarts its own server. Refresh **Endpoint agents > Agent downloads** to see
the available ZIPs. Building Go agents restores GOOS, GOARCH, and CGO_ENABLED
before compiling the server.

Loopback HTTP ZIPs are only for an agent on the server laptop. To manage a separate
computer, configure reachable LAN HTTPS and download from that address as described
in docs/operations/endpoint-agents.md. The separate native Windows installer EXE
still requires HTTPS and CRYOL_WINDOWS_AGENT_INSTALLER pointing to a staged NSIS
installer. Local setup stages ZIP binaries; it does not create that NSIS installer.
