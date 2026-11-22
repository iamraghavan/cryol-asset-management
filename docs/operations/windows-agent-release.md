# Windows agent installer and release

The installer is a native NSIS setup wizard. It collects the reachable HTTPS server address, a masked single-use enrollment token, an optional public CA certificate, and the scan interval. It registers Cryol Endpoint Inventory in Windows Apps & Features. The endpoint service runs in user mode as LocalService, uses Automatic startup at boot, and records service events in Event Viewer. It requires administrator approval for setup. No kernel driver is required.

## Build for a disposable test machine

Use a current supported NSIS compiler and Go toolchain. From the repository root on macOS or Linux, run `installer/windows/agent/build.sh`. The output is `dist/windows-agent/Cryol-Agent-Setup-x64.exe` with a checksum file. If NSIS is outside PATH, set `MAKENSIS` to its executable. Set `CRYOL_WINDOWS_DEFAULT_SERVER` to prefill a deployment’s HTTPS address. Use versioned output artifacts in release automation. On Windows, build `./cmd/cryol-agent` from `apps/server` for amd64, then run MakeNSIS from `installer/windows/agent` with `/DAGENT_EXE=...` and `/DOUTPUT_EXE=...`.

Set `CRYOL_WINDOWS_AGENT_INSTALLER` to the absolute path of the approved installer on the Cryol server. The console offers it under **Endpoint agents → New enrollment → Download Windows x64 installer**. The Windows ZIP is the fallback when no installer is configured. If an unsigned artifact is exposed on a disposable test console, set `CRYOL_WINDOWS_AGENT_INSTALLER_TEST=1` to label it there. The wizard requires an endpoint-facing HTTPS address. For private HTTPS deployment, set `CRYOL_AGENT_SETUP_ORIGIN` and optionally `CRYOL_AGENT_SETUP_CA` to the public certificate so the console can offer it separately. The wizard can take the CA file directly. All agent connections still verify the server certificate.

The one-use token is passed through the elevated installer process environment to its agent child. It is cleared from both processes after transfer, kept out of command-line arguments and logs, and exchanged for an independent endpoint credential. The enrolled credential is stored under the installed agent's protected state directory. Uninstall unregisters the service and preserves local state and server-side history, as documented in the endpoint guide.

## Publisher identity and endpoint protection

The display name, icon, product metadata and Apps & Features publisher label are cosmetic release metadata. Windows UAC uses the Authenticode signature for the verified publisher. Sign the inner `cryol-agent.exe` first with an authorized code-signing certificate. Compile the installer with that signed binary, then sign the final installer. Use a trusted timestamp and verify both signatures on Windows with `signtool verify /pa /v <file>`. Generate SHA-256 checksums after signing. Keep private keys in the authorized signing environment. [Microsoft code-signing options](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options) · [Authenticode timestamping](https://learn.microsoft.com/en-us/windows/win32/seccrypto/time-stamping-authenticode-signatures)

Test enrollment, first inventory, service reboot, UI cancellation, denied UAC, invalid token, certificate errors and uninstall on a Windows test machine with endpoint protection enabled. A valid signature identifies the publisher and does not guarantee that a security product will allow the file. When a tool raises an alert, retain the detection name, path, product version, SHA-256 and relevant event details for your security team or vendor. Keep the unsigned developer bundle off affected endpoints while the alert is being investigated.

## Repair a registered service

Run the new setup wizard on Windows and select **Repair existing Cryol service**. This option restores the Cryol display name, plain-text description, Automatic startup and bounded restart settings, then verifies the service is running. It reuses existing enrollment and requires no new token. It checks the service path and account first. A missing executable or enrollment file stops repair. Review the endpoint protection report if the binary is missing.

Installer file details and Windows Apps & Features use the developer label **Raghavan - Cryol**. UAC verified publisher still depends on a trusted Authenticode signature issued to the signing identity.

## Signed build on GitHub Actions

The **Signed Windows agent setup** workflow is manual and restricted to `main`. It signs the payload before NSIS packages it, signs the resulting setup EXE, verifies trusted Authenticode signatures and timestamps on Windows, and then uploads `cryol-windows-agent-signed-x64`. Regular push builds remain unsigned test artifacts. The signing service is a build-time dependency only.

1. Create an Azure Artifact Signing account, complete identity validation, and create a **Public Trust** profile for distribution to ordinary Windows PCs. Windows shows the certificate's validated publisher identity. The `Raghavan - Cryol` display label in the wizard is separate.
2. Create a GitHub environment named `windows-code-signing`. Limit it to the release branch and add reviewers if available. In Azure, configure a federated workload identity for `repo:iamraghavan/cryol-asset-management:environment:windows-code-signing` and grant the **Artifact Signing Certificate Profile Signer** role at the appropriate scope. This workflow uses OIDC and does not store a signing key in GitHub.
3. Add environment variables `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `CRYOL_SIGNING_ENDPOINT`, `CRYOL_SIGNING_ACCOUNT`, and `CRYOL_SIGNING_PROFILE` to that GitHub environment.
4. Run the workflow from **Actions → Signed Windows agent setup**. After signing and testing the installer on a Windows endpoint with protection enabled, configure the console to distribute the approved signed installer.

For managed internal PCs, an organization can instead use its own approved enterprise code-signing certificate and distribute trust using its endpoint management policy. Sign the agent and installer in the same order on an authorized Windows signing workstation. The Cryol LAN HTTPS CA is for server authentication and is separate from Windows code signing. The NSIS-generated uninstaller is not covered by the installer signature.

[Microsoft Artifact Signing quickstart](https://learn.microsoft.com/en-us/azure/artifact-signing/quickstart) · [GitHub signing integration](https://learn.microsoft.com/en-us/azure/artifact-signing/how-to-signing-integrations) · [Microsoft signing action](https://github.com/Azure/artifact-signing-action)

## Preconfigured LAN installer and diagnostics

Version 0.3.0 adds a **Test connection** button. It performs the same TLS and readiness checks as enrollment before any token is sent. Errors distinguish DNS resolution, timeouts, refused TCP connections and certificate trust or name failures. A successful probe shows the local and remote addresses used. Enter a DNS name, IPv4 address, or bracketed IPv6 address covered by the server certificate. Routing and firewall rules between the two devices still apply.

For an existing registration, choose **Check service status** to see Windows service state, startup mode, exit code and whether the executable and enrollment file are present. **Repair existing Cryol service** restores service settings without changing the installed binary or enrolling again.

A site-specific installer can include the HTTPS server address and public CA:

- Local build: set `CRYOL_WINDOWS_DEFAULT_SERVER` to the server origin and `CRYOL_WINDOWS_PUBLIC_CA` to the public CA PEM before running `installer/windows/agent/build.sh`.
- GitHub test build: manually run **Windows agent setup**, with `server_origin` and `public_ca_pem_base64`. Supply the base64 encoding of the public PEM only. Packaging rejects non-certificate data and certificates that lack CA capability.
- A bundled CA is used within the agent's TLS client. Windows system trust is unchanged. The signed workflow continues to build a generic artifact unless a site-specific signed packaging process is configured. The configured server URL and public CA must be built into the package before signing.

The one-use enrollment token remains separate from reusable downloads. The connection test and service status check do not consume a token.

When serving a verified site-specific installer with an embedded CA, set `CRYOL_WINDOWS_AGENT_INSTALLER_BUNDLED_CA=1`. This tells the console to omit the separate CA download for Windows Setup. It does not add a certificate to an existing EXE.
