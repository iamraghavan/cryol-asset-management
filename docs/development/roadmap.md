# Implementation roadmap

| Milestone | Deliverable | Verification gate |
| --- | --- | --- |
| 1. Foundation | React and Go workspace, PostgreSQL wiring, embedded migrations, readiness API and development scripts | Build and lint both stacks. Apply migrations twice to a disposable PostgreSQL instance and check readiness. |
| 2. Security and organization | Local account bootstrap and recovery, users, Argon2id, sessions, scoped roles and organization administration | Test expiry, CSRF, role changes and cross-department access against PostgreSQL. |
| 3. Interface | Compact shell, breadcrumbs, tables, form controls, dialogs and design tokens | Test keyboard workflows, loading and error states and narrow layouts. |
| 4. Registry | Classification, asset numbering, registry, search, attachments and history | Test concurrent numbering, scoped search, safe file retrieval and state transitions. |
| 5. Custody | Assignment, returns, transfers and custody history | Test simultaneous assignments and transaction rollback. |
| 6. Reports | Scoped KPIs, CSV and XLSX exports, and audit views | Compare reports with known database records and test export permissions. |
| 7. Windows | NSIS, service, database setup, upgrade and repair | Run clean, reuse, fault, repair and removal cases on Windows. |
| 8. Linux | Native packages, systemd, database setup and repair | Run fresh install, restart, upgrade and data preservation cases on supported distributions. |
| 9. Business operations | Requests, approvals, procurement, maintenance, warranties, audits and depreciation | Verify each module against real data and enforce authorization before adding it to navigation. |
| 10. Hardening | Backup and restore, licensing, performance and release checks | Rehearse recovery, disconnected operation, offline activation and upgrades. Measure performance with 100,000 assets and 25 concurrent users. |

## Security and organization implementation order

1. Add identity and organization migrations with singleton profile constraints and foreign keys for internal units.
2. Add a local administrator bootstrap command that reads its password from a non-echoing input. Add Argon2id password verification, bounded login attempts, hashed session tokens, session expiry, logout and password recovery.
3. Add role permissions and scope matching at the service and query boundaries. Cover access between branches and departments before exposing organization records to ordinary users.
4. Add profile settings and management of divisions, branches, departments, designations, employees and locations. Record administrative changes in the audit log.
5. Connect login and organization routes in React. Exercise setup, login, expired sessions, role assignment, and denial of out-of-scope access through the HTTP API and PostgreSQL.

For each milestone, run formatting, static checks and relevant tests before adding its routes to normal navigation. Update the API contract, migrations guide and implementation status alongside changes. Keep platform installer acceptance and recovery rehearsals tied to the actual operating environments.

## Self-hosted console and endpoint inventory

Cryol is a self-hosted web application for one organization per installation. The console shares the existing sign-in and permissions. The first agent milestone collects read-only inventory.

| Stage | Deliverable | Acceptance check |
| --- | --- | --- |
| 1. Console foundations | Organization settings, managed users and scoped permissions for internal units | Deep links load after refresh. Integration tests verify branch and department access boundaries. |
| 2. Physical registry | Locations, employees, asset classification, registry, attachments and custody | Verify authorized CRUD, concurrent assignment integrity and history. |
| 3. Endpoint inventory | Windows and Linux agent, one-time enrollment, per-device identity and read-only inventory snapshots | Verify revoked credentials, duplicate submissions, intermittent connectivity and correct device attribution on both platforms. |
| 4. Software asset management | Software catalog, installations, license entitlements, allocations and reconciliation | Compare installed and entitled quantities from reproducible fixtures, including duplicate observations and stale devices. |
| 5. Operational modules | Procurement, maintenance, warranties, audits, depreciation, reports and exports | Verify module permissions, state transitions and audit provenance. |
| 6. Deployment and operations | Self-hosted environments, recovery, retention, observability, agent packaging and release checks | Rehearse a full installation restore and verify native agent installation, upgrade and removal. |

The active settings route is `/console/settings/organization`. Future record routes include `/console/assets/:assetId`, `/console/devices/:deviceId/software` and `/console/software/entitlements/:entitlementId`. Browser APIs remain under `/api/v1` and enforce authorization for each record. Agents will use separate `/api/agent/v1/enrollments` and `/api/agent/v1/inventory-snapshots` routes. The authenticated device identity will determine device attribution for uploads.

Define the agent payload explicitly for hardware and installed software. Queue snapshots locally during outages with a size limit and retry with backoff. Scope each device credential to inventory submission and permit revocation from the console. Record enrollment and credential changes in the audit log.
