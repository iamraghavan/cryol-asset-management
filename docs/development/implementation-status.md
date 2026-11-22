# Implementation status

The repository started with a README and no existing code or database configuration. Milestone 1 establishes the React and Go workspace, embedded PostgreSQL migrations, typed queries and an API-backed system-status screen. The CLI manages database migrations separately from service startup.

The asset registry, custody, reports, attachments, backup and restore, licensing, and native installers remain roadmap work. Platform support and performance targets require testing on the intended environments.

## Milestone 1 verification

On macOS x86-64 with Go 1.26.4, Node.js 22.15.1 and disposable PostgreSQL 17.9:

- Vite started and served the React entry page.
- The frontend build, ESLint and the Vitest/React Testing Library readiness test passed.
- Go build, vet and HTTP route tests passed.
- The PostgreSQL integration test verified that a fresh database fails readiness, applying migrations makes it ready, and reapplying migrations preserves the installation UUID.
- The live Go service returned liveness, readiness and version responses and served the production HTML and JavaScript.
- The GitHub Actions workflow is defined. Hosted execution and native Windows and Linux installation tests remain pending.
- Go cross-builds for Linux amd64 and Windows amd64 passed on macOS.

## Milestone 2 in progress

Implemented local administrator bootstrap, sign-in, sign-out, server-side session expiry and rotation, permission checks on organization profile access, and audit events for profile writes. Migration 2 adds identity, session, and profile tables. The built application serves sign-in and profile forms at its root.

User administration, account recovery, organizational directory, branch and department access scopes, and the full default role set remain open. Later milestones are pending.

On macOS with PostgreSQL 17.9, the Go integration tests verified a single bootstrap, rejected credentials, secure HTTPS cookie attributes, CSRF rejection, permission revocation, expiry, session rotation and sign-out. The live loopback smoke test ran the compiled CLI and server, created a temporary administrator, served the React entry page, signed in, wrote and read the profile, and signed out. Frontend build, lint and both React tests passed.

## Dashboard and asset registry

The console opens the dashboard after sign-in from the root. Initial organization setup returns to the dashboard after saving. The sidebar links to the dashboard, asset registry, organization settings and system status according to the current user's permissions.

The registry creates assets with a sequence-backed identifier using the saved organization prefix. It lists persisted records in pages of 25. The dashboard reads the total asset count from PostgreSQL. Asset editing, classification, search, custody, device inventory and software management remain open.

## Enterprise console and registry enhancement

The console now has a collapsible Lucide sidebar, breadcrumbs, an account menu, quick asset creation and a keyboard-searchable asset palette. The asset registry uses server-side filtering, sorting and pagination. Users can customize visible columns and export selected rows as CSV. Create, detail, edit and filter use reusable Radix drawers. Asset edits, archive and restore enforce permissions and an optimistic revision. The dashboard derives total, active and archived counts from PostgreSQL.

The current asset form stores identity, manufacturer, model, serial number, notes and purchase date. Financial and warranty fields, attachments, category and location relationships, custody workflows, saved views and multi-entity search remain open. The searchable registry has not been benchmarked on a production-sized dataset.

Verification passed on macOS with PostgreSQL 17.9 and Playwright using Brave's Chromium engine. The browser scenario exercised sign-in, organization setup, asset creation and details, keyboard search, editing, Escape and Back unsaved-change guards, manufacturer and date filters, archive and restoration, selected CSV download and dashboard navigation. Go integration tests verified stale revision conflicts, permission denial and migration compatibility. Frontend build, lint, and Vitest passed. The local instance was backed up and restarted. Existing organization and asset fields matched before and after the migration.

## Endpoint inventory foundation

Migration 5 and the agent console implement enrollment, credential revocation, heartbeat-based status, idempotent snapshots, device-to-asset linking, and paginated installed-software browsing. The Windows and Linux executable provides manual enrollment, a one-shot scan, and a scheduled foreground mode. The latest complete software snapshot remains visible after an unavailable software collection.

The snapshot format is versioned JSON with relational device, agent, enrollment and run identity. A normalized software catalog and component-level current tables, change detection, cross-device reporting and license management remain open, alongside organization and custody modules. The [endpoint operations guide](../operations/endpoint-agents.md) documents setup and collector coverage.

Verified on macOS with PostgreSQL 17.9. Go integration tests covered HTTPS enforcement, one-use enrollment, heartbeat, replay-safe inventory ingestion, conflicts, retention of the last complete software report, explicit linking and unlinking, and credential revocation. Playwright passed console token creation and revocation plus the existing asset workflow. Frontend build, lint and Vitest passed. The Linux and Windows amd64 agent binaries cross-compiled. Their native collectors and background service behavior still need testing on the target systems.

## Windows background agent

The Windows console bundle now requests UAC approval, enrolls using a one-use token, and registers Cryol Endpoint Inventory for delayed automatic startup as LocalService. Windows Event Viewer receives agent logs. The Windows collector adds reported BIOS information, physical memory, and fixed-volume capacity and available space. Linux retains its foreground launcher.

Verified Go checks, disposable PostgreSQL integration tests, Windows amd64 cross-build and vet, frontend checks, Playwright, and the live Windows ZIP download. Windows installation, collector execution, reboot behavior and removal remain unverified on a native host.
