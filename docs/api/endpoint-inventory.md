# Endpoint inventory API

The browser routes use Cryol sessions, CSRF protection and the permissions below. Token generation responds with the plaintext token once. Its digest, expiry and consumption state are stored in PostgreSQL.

| Method and path | Permission or credential |
| --- | --- |
| POST /api/v1/agents/enrollment-tokens | agents.enroll |
| POST /api/v1/agents/enrollment-tokens/{id}/revoke | agents.enroll |
| GET /api/v1/agents?page=1 | agents.read |
| GET /api/v1/agents/{id}/inventory?part=hardware | agents.read and hardware.read |
| GET /api/v1/agents/{id}/inventory?part=software&page=1&q= | agents.read and software.read |
| PUT /api/v1/agents/{id}/asset | devices.manage and assets.read |
| POST /api/v1/agents/{id}/revoke | agents.revoke |
| POST /api/agent/v1/enroll | HTTPS and an unused enrollment bearer token |
| POST /api/agent/v1/heartbeat | HTTPS and the agent's bearer credential |
| POST /api/agent/v1/inventory | HTTPS and the agent's bearer credential |

Enrollment accepts `{"installationId":"<UUID>"}` and returns a per-agent credential. Linking accepts `{"assetId":"<UUID>"}` or `{"assetId":null}` to unlink. Agent and software lists return up to 50 records per page.

Inventory accepts a JSON schema version 1 snapshot, up to 4 MiB, matching the Go contract in `internal/inventory/contract.go`. Resubmitting the same run with the same content succeeds. Reusing the run with different content returns HTTP 409. Revoked agent credentials return HTTP 401. An unavailable software collector does not replace the last complete software view.
