# Current application audit

Baseline: the existing React/Vite console, Go/chi API and PostgreSQL schema through migration 3. This audit distinguishes the prior live verification from features inferred from source.

| Module | Classification | Working baseline | Gaps and risks | Priority |
| --- | --- | --- | --- | --- |
| Authentication | Partially complete | Local sign-in, sign-out, bootstrap, database sessions and permission checks passed prior live and integration checks | No managed users, recovery or branch scopes. Login throttling is process-wide. | High |
| Organization | Partially complete | Singleton profile save/read and first-run dashboard navigation passed | Organization hierarchy, employees and category lookups are missing. | High |
| Dashboard | Partially complete | PostgreSQL total count and direct URL verified | A single metric. Distribution, custody metrics and activity need domain records. | Medium |
| Asset registry | Partially complete | Create, view list and pagination backed by PostgreSQL. Create audit event and unique numbering tested | Name and serial only. Detail, editing, archive, filtering, search and concurrency protection for edits missing. | Critical |
| Console shell | Partially complete | Dashboard, assets, settings and system routes served directly | Sidebar is always expanded. Search, grouped navigation and record drawers missing. | High |
| Categories, employees, branches, departments, locations | Missing | — | No business CRUD or relational constraints. | High |
| Custody, maintenance, procurement, reporting and device inventory | Missing | — | No domain APIs, history or workflows. | High |
| Native installers and recovery | Missing | Go binaries previously cross-built | Platform service registration, installation and backup restore remain unverified. | High |

Runtime stack uses React, Router, Query, plain CSS, Go/chi, pgx and Goose, with sqlc currently used for installation identity. Existing Vitest tests cover system status, login and organization onboarding. Go tests cover migrations, authentication, authorization, CSRF and initial registry creation. The current release workflow runs verification and cross-builds without native packaging.

The initial registry uses offset pagination with a stable index. Search performance has no baseline because search is absent. Audit events encode asset UUIDs in event names. New record history should remain able to read those events. Preserve migrations 1–3 and the existing list/create request contracts.
