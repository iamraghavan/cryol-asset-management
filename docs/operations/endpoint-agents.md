# Endpoint discovery

The initial endpoint workflow collects inventory from authorized Windows and Linux computers. Agent connections require direct HTTPS to the Cryol server. Configure `CRYOL_TLS_CERT` and `CRYOL_TLS_KEY` with a certificate matching the server name. Give the agent the internal CA PEM through `-ca` when the system trust store does not already include it. Enrollment uses an outbound HTTPS connection and a one-use token valid for 15 minutes.

## Operation

1. Sign in as an administrator and open **Endpoint agents**.
2. Generate a token for the computer you intend to enroll.
3. Build the binaries from `apps/server` with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o cryol-agent ./cmd/cryol-agent` or `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o cryol-agent.exe ./cmd/cryol-agent`. Copy the matching binary to the managed endpoint.
4. As the intended service account, run `cryol-agent enroll -server https://cryol.example.internal:8080 -state-dir /var/lib/cryol-agent -ca /path/to/internal-ca.pem`. On Windows use `cryol-agent.exe` and a private directory for the account. Enter the token at the prompt.
5. Run the same command with `scan` for a single scan or `run` for a foreground process with a five-minute heartbeat and a daily scan. Use `-interval` to adjust scan frequency. Use **Endpoint agents** to inspect inventory, link an existing active asset, or revoke a device.

Use a dedicated user and state directory. On Linux the directory must exclude group and other access. On Windows provision a directory whose ACL limits access to the intended service account and authorized administrators. Mode bits alone do not secure Windows files. The state contains a per-device credential and one pending snapshot. The agent uses a local exclusive lock. Windows uses an operating-system file lock that releases when the process exits. If a Linux crash leaves the lock behind, confirm the process has stopped before clearing it. An interrupted enrollment after the server has consumed its token may need revocation and a fresh enrollment. If a pending scan has exceeded the server's 30-day acceptance period, review and remove the expired pending file while the agent is stopped, then scan again.

## Collection coverage

| Platform | Current coverage |
| --- | --- |
| Linux | Hostname, architecture, OS name, CPU description and logical count, memory, readable DMI identity, network adapters and addresses, and installed dpkg or rpm packages |
| Windows | Hostname, architecture, OS registry version fields, CPU description and logical count, system manufacturer and model, reported BIOS vendor/version, physical memory, fixed drive capacity and available space, network adapters and addresses, and machine-wide uninstall entries from 32-bit and 64-bit registry views |

Missing fields are displayed as unknown. Per-user Windows installs, drivers, physical disk and firmware details beyond the values above, peripherals, changes, and software entitlement reconciliation remain pending. Snapshots are retained in PostgreSQL. Automated retention, signed installer packaging, Linux service setup and native execution tests remain open.

References: [Windows Installer uninstall registry key](https://learn.microsoft.com/en-us/windows/win32/msi/uninstall-registry-key), [Debian dpkg-query](https://manpages.debian.org/stable/dpkg/dpkg-query.1.en.html).

## Console setup download

Set `CRYOL_AGENT_DOWNLOAD_DIR` on the Cryol server to an admin-managed directory containing `cryol-agent.exe` for Windows x64 and `cryol-agent` for Linux x64. From **Endpoint agents → New enrollment**, download the ZIP for the endpoint, extract it and generate a token. On Windows double-click **Start Cryol Agent.cmd**, approve the Windows administrator prompt, and paste the token. Setup installs **Cryol Endpoint Inventory** (service name `CryolAgent`) as a delayed automatic Windows Service. The service runs as LocalService, begins the first scan immediately and stays active after setup closes. On Linux run `sh start-cryol-agent.sh` in the extracted directory, paste the token and keep the terminal open. Both console bundles schedule scans about every five minutes. The ZIP contains the agent and a launcher configured for the console address. Enrollment tokens are entered separately.

For an internal CA, install trust in the Windows Local Computer certificate store for service use or put the public CA certificate in the extracted ZIP folder as `cryol-ca.pem`. Use the DNS name or IP address that matches the Cryol server certificate. Open the console using the address the endpoint can reach before downloading its bundle. Changing the origin generates a separate Linux launcher state directory. The Windows installer registers one service per machine. Reconfiguring an installed Windows service for a different server requires deliberate operator action.

For same-computer development, use a loopback listener and set `CRYOL_AGENT_LOCAL_HTTP=1` on the server. Download the bundle from the localhost console and run it on the same Linux or Windows computer. For a Windows or Linux endpoint connecting to a Cryol server on a different computer, including a macOS development server, use HTTPS on a reachable LAN address.

## Windows service operations

Use **Services** to check `Cryol Endpoint Inventory` and **Event Viewer → Windows Logs → Application** for the `CryolAgent` source. Check **Last scan** in Cryol to confirm inventory delivery. Service start and successful upload are separate events. Setup stores the binary, optional CA certificate and private state under `%ProgramFiles%\Cryol Agent`. The service can update its state. The executable directory is writable by administrators and SYSTEM.

To stop scheduled scans, stop the service. To unregister it, run `cryol-agent.exe uninstall-service` from an elevated terminal. The local directory and enrollment are preserved when unregistering. For a fresh install after that, retain the old directory as protected backup or remove it through an authorized cleanup after revoking the old agent in Cryol. Use a new token to enroll again. Existing foreground endpoints should stop their old launcher and revoke or reconcile the old agent before enrolling the service.

The Windows binary was cross-built. Native installation, UAC interaction, startup after reboot and uninstall require verification on Windows before release.

## If enrollment cannot reach localhost

A bundle downloaded through `http://127.0.0.1:8080` points to the machine running that bundle. For a separate Windows PC or VM, open the Cryol console on that endpoint through the server's reachable HTTPS address and download a fresh `cryol-agent-windows-service-x64.zip`. An older ZIP or extracted launcher keeps the address it was generated with.

Bind Cryol to a reachable LAN IP and set `CRYOL_TLS_CERT` and `CRYOL_TLS_KEY`. The TLS certificate must cover the exact DNS name or IP used in the console URL. On Windows, open `https://HOST:PORT/api/v1/system/readiness` in a browser before downloading. If the browser cannot reach it, check the server listener, private-network firewall, VM networking and certificate trust. Matching the certificate and trusting its issuer are separate checks. The new launcher checks readiness before prompting for the one-use token.
