package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errAgentConflict = errors.New("agent inventory conflict")

func agentSecret() (string, [32]byte) {
	var raw [32]byte
	rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:]), sha256.Sum256(raw[:])
}

func bearerDigest(r *http.Request) ([32]byte, bool) {
	token, exists := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !exists || len(token) != 43 {
		return [32]byte{}, false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return sha256.Sum256(raw), err == nil && len(raw) == 32
}

func MountAgents(r chi.Router, pool *pgxpool.Pool) {
	r.Group(func(r chi.Router) {
		r.Use(http.NewCrossOriginProtection().Handler)
		r.Get("/api/v1/agents/setup", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, pool, "agents.enroll"); ok {
				setupOptions(w, r)
			}
		})
		r.Get("/api/v1/agents/setup/windows-installer", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, pool, "agents.enroll"); ok {
				serveWindowsInstaller(w, r)
			}
		})
		r.Get("/api/v1/agents/setup/ca", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, pool, "agents.enroll"); ok {
				serveSetupCA(w, r)
			}
		})
		r.Get("/api/v1/agents/setup/{platform}", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, pool, "agents.enroll"); ok {
				serveSetup(w, r)
			}
		})
		r.Post("/api/v1/agents/enrollment-tokens", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "agents.enroll")
			if !ok {
				return
			}
			token, digest := agentSecret()
			var id string
			var expires time.Time
			err := pool.QueryRow(r.Context(), `WITH entry AS (
INSERT INTO agent_enrollment(token_digest, created_by, expires_at)
VALUES ($1,$2,now()+interval '15 minutes') RETURNING id,expires_at
), audit AS (INSERT INTO audit_log(actor_id,event) SELECT $2,'agent.enrollment.issued:'||id::text FROM entry)
SELECT id::text,expires_at FROM entry`, digest[:], user.ID).Scan(&id, &expires)
			if err != nil {
				agentUnavailable(w, r)
				return
			}
			reply(w, 201, map[string]any{"id": id, "token": token, "expiresAt": expires})
		})
		r.Get("/api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "agents.read")
			if !ok {
				return
			}
			listAgents(w, r, pool, user.Can("assets.read"), user.Can("hardware.read"))
		})
		r.Get("/api/v1/agents/{id}/inventory", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "agents.read")
			if !ok {
				return
			}
			part := r.URL.Query().Get("part")
			switch {
			case part == "hardware" && user.Can("hardware.read"), part == "software" && user.Can("software.read"):
				readInventory(w, r, pool, part)
			default:
				fail(w, r, 403, "INVENTORY_ACCESS_DENIED", "You do not have access to this inventory section.")
			}
		})
		r.Get("/api/v1/agents/{id}/inventory-history", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "agents.read")
			if !ok {
				return
			}
			if !user.Can("hardware.read") && !user.Can("software.read") {
				fail(w, r, 403, "INVENTORY_ACCESS_DENIED", "You do not have access to inventory history.")
				return
			}
			readInventoryHistory(w, r, pool, user.Can("software.read"))
		})
		r.Put("/api/v1/agents/{id}/asset", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "devices.manage")
			if !ok {
				return
			}
			if !user.Can("assets.read") {
				fail(w, r, 403, "ASSET_ACCESS_DENIED", "Asset read permission is required to link devices.")
				return
			}
			linkAgentAsset(w, r, pool, user.ID)
		})
		r.Post("/api/v1/agents/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "agents.revoke")
			if !ok {
				return
			}
			revokeAgent(w, r, pool, user.ID, false)
		})
		r.Post("/api/v1/agents/enrollment-tokens/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "agents.enroll")
			if !ok {
				return
			}
			revokeAgent(w, r, pool, user.ID, true)
		})
	})
	r.Route("/api/agent/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.TLS == nil && !agentLoopbackHTTP(r) {
					fail(w, r, 403, "TLS_REQUIRED", "Agent connections require HTTPS.")
					return
				}
				next.ServeHTTP(w, r)
			})
		})
		r.Post("/enroll", func(w http.ResponseWriter, r *http.Request) { enrollAgent(w, r, pool) })
		r.Post("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
			hash, ok := bearerDigest(r)
			if !ok {
				invalidAgent(w, r)
				return
			}
			tag, err := pool.Exec(r.Context(), "UPDATE endpoint_agent SET last_seen=now() WHERE token_digest=$1 AND revoked_at IS NULL", hash[:])
			if err != nil {
				agentUnavailable(w, r)
				return
			}
			if tag.RowsAffected() == 0 {
				invalidAgent(w, r)
				return
			}
			reply(w, 200, map[string]string{"status": "accepted"})
		})
		r.Post("/inventory", func(w http.ResponseWriter, r *http.Request) { writeInventory(w, r, pool) })
	})
}

func enrollAgent(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	hash, ok := bearerDigest(r)
	if !ok {
		invalidAgent(w, r)
		return
	}
	var input struct {
		InstallationID string `json:"installationId"`
	}
	if !decode(w, r, &input) {
		return
	}
	var id pgtype.UUID
	if id.Scan(input.InstallationID) != nil || !id.Valid || id.Bytes == [16]byte{} {
		fail(w, r, 422, "INVALID_INSTALLATION", "A valid installation identifier is required.")
		return
	}
	credential, digest := agentSecret()
	ctx := r.Context()
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE agent_enrollment SET consumed_at=now()
WHERE token_digest=$1 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>now()`, hash[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		var device string
		if err := tx.QueryRow(ctx, "INSERT INTO inventory_device DEFAULT VALUES RETURNING id::text").Scan(&device); err != nil {
			return err
		}
		tag, err = tx.Exec(ctx, `INSERT INTO endpoint_agent(id,device_id,token_digest) VALUES ($1,$2,$3) ON CONFLICT (id) DO NOTHING`, id, device, digest[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errAgentConflict
		}
		_, err = tx.Exec(ctx, "INSERT INTO audit_log(event) VALUES ($1)", "agent.enrolled:"+input.InstallationID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		invalidAgent(w, r)
		return
	}
	if errors.Is(err, errAgentConflict) {
		fail(w, r, 409, "INSTALLATION_EXISTS", "This installation has already enrolled.")
		return
	}
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	reply(w, 201, map[string]string{"credential": credential})
}

func writeInventory(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	hash, ok := bearerDigest(r)
	if !ok {
		invalidAgent(w, r)
		return
	}
	ctx := r.Context()
	var known bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM endpoint_agent WHERE token_digest=$1 AND revoked_at IS NULL)", hash[:]).Scan(&known); err != nil {
		agentUnavailable(w, r)
		return
	}
	if !known {
		invalidAgent(w, r)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, inventory.MaxReportBytes))
	if err != nil {
		fail(w, r, 413, "INVENTORY_TOO_LARGE", "The inventory exceeds the upload limit.")
		return
	}
	var snap inventory.Snapshot
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var run pgtype.UUID
	if decoder.Decode(&snap) != nil || decoder.Decode(new(json.RawMessage)) != io.EOF || snap.Validate(time.Now()) != nil || run.Scan(snap.RunID) != nil || !run.Valid || run.Bytes == [16]byte{} {
		fail(w, r, 422, "INVALID_INVENTORY", "The inventory does not match the supported schema.")
		return
	}
	canonical, err := json.Marshal(snap)
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	digest := sha256.Sum256(canonical)
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var agent, device string
		var last *time.Time
		if err := tx.QueryRow(ctx, `SELECT id::text,device_id::text,last_inventory_at FROM endpoint_agent
WHERE token_digest=$1 AND revoked_at IS NULL FOR UPDATE`, hash[:]).Scan(&agent, &device, &last); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO inventory_snapshot(agent_id,run_id,payload,digest,scanned_at)
VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, agent, run, canonical, digest[:], snap.ScannedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var identical bool
			err := tx.QueryRow(ctx, "SELECT digest=$3 FROM inventory_snapshot WHERE agent_id=$1 AND run_id=$2", agent, run, digest[:]).Scan(&identical)
			if err != nil {
				return err
			}
			if !identical {
				return errAgentConflict
			}
			return nil
		}
		if last == nil || !snap.ScannedAt.Before(*last) {
			if _, err := tx.Exec(ctx, `UPDATE inventory_device SET hostname=$2,os_family=$3,architecture=$4 WHERE id=$1`, device, snap.Hostname, snap.OSFamily, snap.Architecture); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "UPDATE endpoint_agent SET last_seen=now(),last_inventory_at=$2,version=$3 WHERE id=$1", agent, snap.ScannedAt, snap.AgentVersion)
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE endpoint_agent SET last_seen=now() WHERE id=$1", agent)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		invalidAgent(w, r)
		return
	}
	if errors.Is(err, errAgentConflict) {
		fail(w, r, 409, "RUN_CONFLICT", "This scan identifier already exists with different inventory.")
		return
	}
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	reply(w, 200, map[string]string{"status": "accepted"})
}

func readAgentID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if id.Scan(chi.URLParam(r, "id")) != nil || !id.Valid {
		fail(w, r, 400, "INVALID_AGENT_ID", "Invalid agent identifier.")
		return id, false
	}
	return id, true
}

func agentsPage(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("page")
	if raw == "" {
		return 1, true
	}
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 || page > 100000 {
		fail(w, r, 400, "INVALID_PAGE", "Page must be between 1 and 100000.")
		return 0, false
	}
	return page, true
}

func listAgents(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, seeAssets, seeHardware bool) {
	page, ok := agentsPage(w, r)
	if !ok {
		return
	}
	rows, err := pool.Query(r.Context(), `SELECT a.id::text,d.id::text,d.hostname,d.os_family,d.architecture,a.version,a.last_seen,a.last_inventory_at,coalesce(d.asset_id::text,''),coalesce(asset.asset_number,''),
coalesce(latest.os_name,''),coalesce(latest.serial,''),
CASE WHEN a.revoked_at IS NOT NULL THEN 'revoked' WHEN a.last_seen>now()-interval '15 minutes' THEN 'online' ELSE 'offline' END
FROM (SELECT id,device_id,version,last_seen,last_inventory_at,revoked_at FROM endpoint_agent ORDER BY last_seen DESC,id LIMIT 51 OFFSET $1) a
JOIN inventory_device d ON d.id=a.device_id LEFT JOIN asset ON asset.id=d.asset_id
LEFT JOIN LATERAL (
  SELECT payload->>'osName' AS os_name, payload->>'serial' AS serial
  FROM inventory_snapshot WHERE $2::boolean AND agent_id=a.id
  ORDER BY scanned_at DESC, received_at DESC, run_id DESC LIMIT 1
) latest ON true
ORDER BY a.last_seen DESC,a.id`, (page-1)*50, seeHardware)
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (map[string]any, error) {
		var id, device, host, family, arch, version, status, assetID, assetNumber, osName, serial string
		var seen time.Time
		var scanned *time.Time
		err := row.Scan(&id, &device, &host, &family, &arch, &version, &seen, &scanned, &assetID, &assetNumber, &osName, &serial, &status)
		if !seeAssets {
			assetID, assetNumber = "", ""
		}
		return map[string]any{"id": id, "deviceId": device, "hostname": host, "osFamily": family, "osName": osName, "serial": serial, "architecture": arch, "version": version, "lastSeen": seen, "lastInventoryAt": scanned, "status": status, "assetId": assetID, "assetNumber": assetNumber}, err
	})
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	next := len(items) > 50 && page < 100000
	if len(items) > 50 {
		items = items[:50]
	}
	reply(w, 200, map[string]any{"items": items, "hasNext": next})
}

func readInventoryHistory(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, seeSoftware bool) {
	id, ok := readAgentID(w, r)
	if !ok {
		return
	}
	page, ok := agentsPage(w, r)
	if !ok {
		return
	}
	var exists bool
	if err := pool.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM endpoint_agent WHERE id=$1)", id).Scan(&exists); err != nil {
		agentUnavailable(w, r)
		return
	}
	if !exists {
		fail(w, r, 404, "AGENT_NOT_FOUND", "This agent could not be found.")
		return
	}
	rows, err := pool.Query(r.Context(), `SELECT run_id::text, scanned_at, received_at,
coalesce(payload->>'agentVersion',''), CASE WHEN $3::boolean THEN coalesce(payload->>'softwareStatus','') ELSE '' END
FROM inventory_snapshot WHERE agent_id=$1 ORDER BY scanned_at DESC,received_at DESC,run_id DESC LIMIT 51 OFFSET $2`, id, (page-1)*50, seeSoftware)
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	type scan struct {
		RunID          string    `json:"runId"`
		ScannedAt      time.Time `json:"scannedAt"`
		ReceivedAt     time.Time `json:"receivedAt"`
		AgentVersion   string    `json:"agentVersion"`
		SoftwareStatus string    `json:"softwareStatus,omitempty"`
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (scan, error) {
		var item scan
		err := row.Scan(&item.RunID, &item.ScannedAt, &item.ReceivedAt, &item.AgentVersion, &item.SoftwareStatus)
		return item, err
	})
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	next := len(items) > 50 && page < 100000
	if len(items) > 50 {
		items = items[:50]
	}
	reply(w, 200, map[string]any{"items": items, "hasNext": next})
}

func readInventory(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, part string) {
	id, ok := readAgentID(w, r)
	if !ok {
		return
	}
	query := `SELECT payload - 'software' FROM inventory_snapshot WHERE agent_id=$1
ORDER BY scanned_at DESC, received_at DESC, run_id DESC LIMIT 1`
	args := []any{id}
	if part == "software" {
		page, ok := agentsPage(w, r)
		if !ok {
			return
		}
		filter := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(filter) > 120 || strings.ContainsRune(filter, 0) {
			fail(w, r, 400, "INVALID_FILTER", "Enter a software search of up to 120 bytes.")
			return
		}
		query = `WITH snapshot AS (
SELECT payload FROM inventory_snapshot WHERE agent_id=$1 AND payload->>'softwareStatus'='complete'
ORDER BY scanned_at DESC, received_at DESC, run_id DESC LIMIT 1
), matches AS (
SELECT entry FROM snapshot, jsonb_array_elements(coalesce(payload->'software','[]'::jsonb)) entry
WHERE strpos(lower(concat_ws(' ',entry->>'name',entry->>'publisher',entry->>'version')),lower($2))>0 OR $2=''
ORDER BY lower(entry->>'name'), entry->>'source',entry->>'id'
LIMIT 51 OFFSET $3
), numbered AS (SELECT entry, row_number() OVER () position FROM matches)
SELECT jsonb_build_object('software',coalesce((SELECT jsonb_agg(entry ORDER BY position) FROM numbered WHERE position<=50),'[]'::jsonb),
'scannedAt',payload->'scannedAt','hasNext',(SELECT count(*)>50 FROM numbered)) FROM snapshot`
		args = append(args, filter, (page-1)*50)
	}
	var data json.RawMessage
	err := pool.QueryRow(r.Context(), query, args...).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, r, 404, "INVENTORY_MISSING", "No inventory is available for this section.")
		return
	}
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	reply(w, 200, data)
}

func revokeAgent(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, actor string, token bool) {
	id, ok := readAgentID(w, r)
	if !ok {
		return
	}
	query := `WITH changed AS (UPDATE endpoint_agent SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL RETURNING id)
INSERT INTO audit_log(actor_id,event) SELECT $2,'agent.revoked:'||id::text FROM changed`
	if token {
		query = `WITH changed AS (UPDATE agent_enrollment SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL AND consumed_at IS NULL RETURNING id)
INSERT INTO audit_log(actor_id,event) SELECT $2,'agent.enrollment.revoked:'||id::text FROM changed`
	}
	tag, err := pool.Exec(r.Context(), query, id, actor)
	if err != nil {
		agentUnavailable(w, r)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, r, 404, "REVOCATION_UNAVAILABLE", "This enrollment or agent is no longer available to revoke.")
		return
	}
	reply(w, 200, map[string]string{"status": "revoked"})
}

func invalidAgent(w http.ResponseWriter, r *http.Request) {
	fail(w, r, 401, "INVALID_AGENT_CREDENTIAL", "The agent credential is invalid, revoked or expired.")
}
func agentUnavailable(w http.ResponseWriter, r *http.Request) {
	fail(w, r, 503, "AGENT_INVENTORY_UNAVAILABLE", "The agent service is temporarily unavailable.")
}

func linkAgentAsset(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, actor string) {
	id, ok := readAgentID(w, r)
	if !ok {
		return
	}
	var input struct {
		AssetID *string `json:"assetId"`
	}
	if !decode(w, r, &input) {
		return
	}
	var asset pgtype.UUID
	if input.AssetID != nil && (asset.Scan(*input.AssetID) != nil || !asset.Valid) {
		fail(w, r, 422, "INVALID_ASSET_ID", "Choose a valid asset.")
		return
	}
	ctx := r.Context()
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if asset.Valid {
			var active bool
			err := tx.QueryRow(ctx, "SELECT archived_at IS NULL FROM asset WHERE id=$1 FOR SHARE", asset).Scan(&active)
			if errors.Is(err, pgx.ErrNoRows) {
				return errAgentConflict
			}
			if err != nil {
				return err
			}
			if !active {
				return errAgentConflict
			}
		}
		var device string
		if err := tx.QueryRow(ctx, "SELECT device_id::text FROM endpoint_agent WHERE id=$1", id).Scan(&device); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE inventory_device SET asset_id=$2 WHERE id=$1 AND asset_id IS DISTINCT FROM $2", device, asset)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		action := "device.asset.unlinked:" + device
		if asset.Valid {
			action = "device.asset.linked:" + device + ":" + *input.AssetID
		}
		_, err = tx.Exec(ctx, "INSERT INTO audit_log(actor_id,event) VALUES ($1,$2)", actor, action)
		return err
	})
	var constraint *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		fail(w, r, 404, "AGENT_NOT_FOUND", "The agent could not be found.")
	case errors.Is(err, errAgentConflict), errors.As(err, &constraint) && constraint.Code == "23505":
		fail(w, r, 409, "ASSET_LINK_CONFLICT", "Choose an active asset that is not linked to another endpoint.")
	case err != nil:
		agentUnavailable(w, r)
	default:
		reply(w, 200, map[string]string{"status": "saved"})
	}
}
