package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type assetFields struct {
	Name         string `json:"name"`
	Serial       string `json:"serial"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Notes        string `json:"notes"`
	PurchaseDate string `json:"purchaseDate"`
}

type assetRecord struct {
	ID     string `json:"id"`
	Number string `json:"number"`
	assetFields
	CreatedAt  time.Time  `json:"createdAt"`
	ArchivedAt *time.Time `json:"archivedAt"`
	Revision   int        `json:"revision"`
}

const assetColumns = "id::text, asset_number, name, serial_number, manufacturer, model, notes, coalesce(to_char(purchase_date, 'YYYY-MM-DD'), ''), created_at, archived_at, revision"
const assetListColumns = "id::text, asset_number, name, serial_number, manufacturer, model, ''::text, coalesce(to_char(purchase_date, 'YYYY-MM-DD'), ''), created_at, archived_at, revision"

func scanAsset(row pgx.Row) (assetRecord, error) {
	var item assetRecord
	err := row.Scan(&item.ID, &item.Number, &item.Name, &item.Serial, &item.Manufacturer, &item.Model,
		&item.Notes, &item.PurchaseDate, &item.CreatedAt, &item.ArchivedAt, &item.Revision)
	return item, err
}

func MountAssets(r chi.Router, pool *pgxpool.Pool) {
	r.Group(func(r chi.Router) {
		r.Use(http.NewCrossOriginProtection().Handler)
		r.Get("/api/v1/dashboard/summary", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, pool, "assets.read"); !ok {
				return
			}
			var total, archived int64
			if err := pool.QueryRow(r.Context(), "SELECT count(*), count(archived_at) FROM asset").Scan(&total, &archived); err != nil {
				fail(w, r, 503, "DASHBOARD_UNAVAILABLE", "The asset summary is temporarily unavailable.")
				return
			}
			reply(w, 200, map[string]int64{"totalAssets": total, "activeAssets": total - archived, "archivedAssets": archived})
		})
		r.Get("/api/v1/assets", func(w http.ResponseWriter, r *http.Request) { listAssets(w, r, pool) })
		r.Post("/api/v1/assets", func(w http.ResponseWriter, r *http.Request) { writeAsset(w, r, pool, true) })
		r.Get("/api/v1/assets/{id}", func(w http.ResponseWriter, r *http.Request) { assetDetails(w, r, pool) })
		r.Patch("/api/v1/assets/{id}", func(w http.ResponseWriter, r *http.Request) { writeAsset(w, r, pool, false) })
		r.Post("/api/v1/assets/{id}/archive", func(w http.ResponseWriter, r *http.Request) { changeArchive(w, r, pool, true) })
		r.Post("/api/v1/assets/{id}/restore", func(w http.ResponseWriter, r *http.Request) { changeArchive(w, r, pool, false) })
	})
}

func listAssets(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	if _, ok := authorize(w, r, pool, "assets.read"); !ok {
		return
	}
	params := r.URL.Query()
	page := 1
	if raw := params.Get("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100000 {
			fail(w, r, 400, "INVALID_PAGE", "Page must be between 1 and 100000.")
			return
		}
		page = value
	}
	q := strings.TrimSpace(params.Get("q"))
	state := params.Get("state")
	sort := params.Get("sort")
	manufacturer := strings.TrimSpace(params.Get("manufacturer"))
	from, fromOK := optionalAssetDate(params.Get("from"))
	to, toOK := optionalAssetDate(params.Get("to"))
	orders := map[string]string{"": "created_at DESC, id DESC", "newest": "created_at DESC, id DESC", "oldest": "created_at, id", "name": "lower(name), id", "number": "asset_number, id"}
	order, validSort := orders[sort]
	if utf8.RuneCountInString(q) > 120 || strings.ContainsRune(q, 0) || utf8.RuneCountInString(manufacturer) > 120 || strings.ContainsRune(manufacturer, 0) ||
		!fromOK || !toOK || (from.Valid && to.Valid && from.Time.After(to.Time)) || !validSort || (state != "" && state != "all" && state != "active" && state != "archived") {
		fail(w, r, 400, "INVALID_ASSET_FILTER", "Check the search, status or sort selection.")
		return
	}
	const pageSize = 25
	rows, err := pool.Query(r.Context(), "SELECT "+assetListColumns+` FROM asset
WHERE ($1 = '' OR strpos(lower(asset_number), lower($1)) > 0 OR strpos(lower(name), lower($1)) > 0 OR strpos(lower(serial_number), lower($1)) > 0)
AND ($2 IN ('','all') OR ($2='active' AND archived_at IS NULL) OR ($2='archived' AND archived_at IS NOT NULL))
AND ($3 = '' OR strpos(lower(manufacturer), lower($3)) > 0)
AND ($4::date IS NULL OR purchase_date >= $4)
AND ($5::date IS NULL OR purchase_date <= $5)
ORDER BY `+order+" LIMIT $6 OFFSET $7", q, state, manufacturer, from, to, pageSize+1, (page-1)*pageSize)
	if err != nil {
		fail(w, r, 503, "ASSETS_UNAVAILABLE", "Asset records are temporarily unavailable.")
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (assetRecord, error) { return scanAsset(row) })
	if err != nil {
		fail(w, r, 503, "ASSETS_UNAVAILABLE", "Asset records are temporarily unavailable.")
		return
	}
	hasNext := len(items) > pageSize && page < 100000
	if len(items) > pageSize {
		items = items[:pageSize]
	}
	if items == nil {
		items = []assetRecord{}
	}
	reply(w, 200, map[string]any{"items": items, "hasNext": hasNext, "page": page})
}

func readAssetID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if id.Scan(chi.URLParam(r, "id")) != nil || !id.Valid {
		fail(w, r, 400, "INVALID_ASSET_ID", "The asset identifier is invalid.")
		return id, false
	}
	return id, true
}

func assetDetails(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	if _, ok := authorize(w, r, pool, "assets.read"); !ok {
		return
	}
	id, ok := readAssetID(w, r)
	if !ok {
		return
	}
	item, err := scanAsset(pool.QueryRow(r.Context(), "SELECT "+assetColumns+" FROM asset WHERE id=$1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, r, 404, "ASSET_NOT_FOUND", "The asset could not be found.")
		return
	}
	if err != nil {
		fail(w, r, 503, "ASSETS_UNAVAILABLE", "Asset details are temporarily unavailable.")
		return
	}
	rows, err := pool.Query(r.Context(), `SELECT split_part(event, ':', 1), occurred_at FROM audit_log
WHERE event IN ('asset.created:' || $1, 'asset.updated:' || $1, 'asset.archived:' || $1, 'asset.restored:' || $1)
ORDER BY id DESC LIMIT 25`, item.ID)
	if err != nil {
		fail(w, r, 503, "ASSETS_UNAVAILABLE", "Asset history is temporarily unavailable.")
		return
	}
	type activity struct {
		Action string    `json:"action"`
		At     time.Time `json:"at"`
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (activity, error) {
		var event activity
		err := row.Scan(&event.Action, &event.At)
		return event, err
	})
	if err != nil {
		fail(w, r, 503, "ASSETS_UNAVAILABLE", "Asset history is temporarily unavailable.")
		return
	}
	reply(w, 200, struct {
		assetRecord
		History []activity `json:"history"`
	}{item, events})
}

func writeAsset(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, create bool) {
	permission := "assets.update"
	if create {
		permission = "assets.create"
	}
	user, ok := authorize(w, r, pool, permission)
	if !ok {
		return
	}
	var input struct {
		assetFields
		Revision int `json:"revision"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Serial = strings.TrimSpace(input.Serial)
	input.Manufacturer = strings.TrimSpace(input.Manufacturer)
	input.Model = strings.TrimSpace(input.Model)
	input.Notes = strings.TrimSpace(input.Notes)
	fields := []string{input.Name, input.Serial, input.Manufacturer, input.Model, input.Notes}
	limits := []int{200, 120, 120, 120, 4000}
	for i, value := range fields {
		if utf8.RuneCountInString(value) > limits[i] || strings.ContainsRune(value, 0) {
			fail(w, r, 422, "INVALID_ASSET", "An asset field exceeds its allowed length or contains an invalid character.")
			return
		}
	}
	if input.Name == "" || (!create && input.Revision < 1) {
		fail(w, r, 422, "INVALID_ASSET", "An asset name and a valid record revision are required for editing.")
		return
	}
	purchase, validDate := optionalAssetDate(input.PurchaseDate)
	if !validDate {
		fail(w, r, 422, "INVALID_PURCHASE_DATE", "Enter a valid purchase date.")
		return
	}
	ctx := r.Context()
	args := []any{input.Name, input.Serial, input.Manufacturer, input.Model, input.Notes, purchase, user.ID}
	if create {
		item, err := scanAsset(pool.QueryRow(ctx, `WITH number AS (
SELECT nextval('asset_number_seq') AS value, asset_prefix FROM organization_profile WHERE singleton
), changed AS (
INSERT INTO asset (asset_number,name,serial_number,manufacturer,model,notes,purchase_date)
SELECT asset_prefix || '-' || lpad(value::text, greatest(6,length(value::text)), '0'),$1,$2,$3,$4,$5,$6 FROM number RETURNING *
), audited AS (INSERT INTO audit_log (actor_id,event) SELECT $7,'asset.created:' || id::text FROM changed)
SELECT `+assetColumns+" FROM changed", args...))
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, r, 409, "ORGANIZATION_REQUIRED", "Set up the organization before adding assets.")
			return
		}
		respondAssetChange(w, r, item, err, 201)
		return
	}
	id, ok := readAssetID(w, r)
	if !ok {
		return
	}
	item, err := scanAsset(pool.QueryRow(ctx, `WITH changed AS (
UPDATE asset SET name=$1,serial_number=$2,manufacturer=$3,model=$4,notes=$5,purchase_date=$6,revision=revision+1
WHERE id=$8 AND revision=$9 AND archived_at IS NULL RETURNING *
), audited AS (INSERT INTO audit_log (actor_id,event) SELECT $7,'asset.updated:' || id::text FROM changed)
SELECT `+assetColumns+" FROM changed", append(args, id, input.Revision)...))
	respondAssetChange(w, r, item, err, 200)
}

func changeArchive(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, archive bool) {
	user, ok := authorize(w, r, pool, "assets.archive")
	if !ok {
		return
	}
	id, ok := readAssetID(w, r)
	if !ok {
		return
	}
	var input struct{ Revision int }
	if !decode(w, r, &input) {
		return
	}
	if input.Revision < 1 {
		fail(w, r, 422, "INVALID_REVISION", "A valid record revision is required.")
		return
	}
	event := "asset.restored:"
	if archive {
		event = "asset.archived:"
	}
	item, err := scanAsset(pool.QueryRow(r.Context(), `WITH changed AS (
UPDATE asset SET archived_at=CASE WHEN $3::boolean THEN now() ELSE NULL END, revision=revision+1
WHERE id=$1 AND revision=$2 AND (archived_at IS NULL)=$3 RETURNING *
), audited AS (INSERT INTO audit_log (actor_id,event) SELECT $4,$5::text || id::text FROM changed)
SELECT `+assetColumns+" FROM changed", id, input.Revision, archive, user.ID, event))
	respondAssetChange(w, r, item, err, 200)
}

func respondAssetChange(w http.ResponseWriter, r *http.Request, item assetRecord, err error, status int) {
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, r, 409, "ASSET_CHANGED", "This record changed or is no longer available. Refresh its details and try again.")
		return
	}
	if err != nil {
		fail(w, r, 503, "ASSET_WRITE_FAILED", "The asset could not be saved.")
		return
	}
	reply(w, status, item)
}

func optionalAssetDate(raw string) (pgtype.Date, bool) {
	if raw == "" {
		return pgtype.Date{}, true
	}
	value, err := time.Parse("2006-01-02", raw)
	if err != nil || value.Year() < 1 || len(raw) != 10 {
		return pgtype.Date{}, false
	}
	return pgtype.Date{Time: value, Valid: true}, true
}
