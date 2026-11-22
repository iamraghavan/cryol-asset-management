# Endpoint discovery

The initial endpoint workflow collects inventory from authorized Windows and Linux computers. Agent connections require direct HTTPS to the Cryol server. Configure `CRYOL_TLS_CERT` and `CRYOL_TLS_KEY` with a certificate matching the server name. Give the agent the internal CA PEM through `-ca` when the system trust store does not already include it. Enrollment uses an outbound HTTPS connection and a one-use token valid for 15 minutes.

## Operation

1. Sign in as an administrator and open **Endpoint agents**.
2. Generate a token for the computer you intend to enroll.
3. Build the binaries from `apps/server` with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o cryol-agent ./cmd/cryol-agent` or `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o cryol-agent.exe ./cmd/cryol-agent`. Copy the matching binary to the managed endpoint.
4. As the intended service account, run `cryol-agent enroll -server https://cryol.example.internal:8080 -state-dir /var/lib/cryol-agent -ca /path/to/internal-ca.pem`. On Windows use `cryol-agent.exe` and a private directory for the account. Enter the token at the prompt.
5. Run the same command with `scan` for a single scan or `run` for a foreground process with a five-minute heartbeat and a daily scan. Use `-interval` to adjust scan frequency. Use **Endpoint agents** to inspect inventory, link an existing active asset, or revoke a device.

Use a dedicated user and state directory. On Linux the directory must exclude group and other access. On Windows provision a directory whose ACL limits access to the intended service account and authorized administrators. Mode bits alone do not secure Windows files. The state contains a per-device credential and one pending snapshot. The agent uses a local exclusive lock. If a process crash leaves the lock behind, confirm the process has stopped before clearing it. An interrupted enrollment after the server has consumed its token may need revocation and a fresh enrollment. If a pending scan has exceeded the server's 30-day acceptance period, review and remove the expired pending file while the agent is stopped, then scan again.

## Collection coverage

| Platform | Current coverage |
| --- | --- |
| Linux | Hostname, architecture, OS name, CPU description and logical count, memory, readable DMI identity, network adapters and addresses, and installed dpkg or rpm packages |
| Windows | Hostname, architecture, OS registry version fields, CPU description and logical count, system manufacturer and model, network adapters and addresses, and machine-wide uninstall entries from 32-bit and 64-bit registry views |

Missing fields are displayed as unknown. Per-user Windows installs, drivers, component-level storage and firmware, peripherals, changes, and software entitlement reconciliation remain pending. Snapshots are retained in PostgreSQL. Automated retention, native service registration, installer packaging and native execution tests remain open.

References: [Windows Installer uninstall registry key](https://learn.microsoft.com/en-us/windows/win32/msi/uninstall-registry-key), [Debian dpkg-query](https://manpages.debian.org/stable/dpkg/dpkg-query.1.en.html).

## Console setup download

Set `CRYOL_AGENT_DOWNLOAD_DIR` on the Cryol server to an admin-managed directory containing `cryol-agent.exe` for Windows x64 and `cryol-agent` for Linux x64. From **Endpoint agents → New enrollment**, download the ZIP for the endpoint, extract it and generate a token. On Windows double-click **Start Cryol Agent.cmd**. On Linux run `sh start-cryol-agent.sh` in the extracted directory. Paste the token when prompted. The initial scan starts immediately and later scans run about every five minutes while the window stays open. Restarting the same launcher reuses the enrolled identity. The ZIP contains the agent and a launcher configured for the console address. Enrollment tokens are entered separately.

For an internal CA, install trust on the endpoint or put the public CA certificate in the extracted ZIP folder as `cryol-ca.pem`. Use the DNS name or IP address that matches the Cryol server certificate. Open the console using the address the endpoint can reach before downloading its bundle. Changing the origin generates a separate local agent state location.

For same-computer development, use a loopback listener and set `CRYOL_AGENT_LOCAL_HTTP=1` on the server. Download the bundle from the localhost console and run it on the same Linux or Windows computer. For a Windows or Linux endpoint connecting to a Cryol server on a different computer, including a macOS development server, use HTTPS on a reachable LAN address.
