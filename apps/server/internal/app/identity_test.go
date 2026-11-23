package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/auth"
	"github.com/cryol/cryol-asset-management/apps/server/internal/database"
	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"github.com/go-chi/chi/v5"
)

func TestIdentityAndOrganization(t *testing.T) {
	url := os.Getenv("CRYOL_IDENTITY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CRYOL_IDENTITY_TEST_DATABASE_URL requires a separate disposable empty database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	password := "test account temporary passphrase"
	if err := auth.Bootstrap(ctx, pool, "test-admin", []byte(password)); err != nil {
		t.Fatal(err)
	}
	if err := auth.Bootstrap(ctx, pool, "other-admin", []byte(password)); err != auth.ErrInitialized {
		t.Fatalf("duplicate bootstrap returned %v", err)
	}

	handler := Handler(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, "test", "", func(r chi.Router) {
		MountIdentity(r, pool)
		MountAssets(r, pool)
		MountAgents(r, pool)
	})
	var cookie *http.Cookie
	call := func(method, path, payload, origin string, want int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "https://cryol.test/api/v1"+path, strings.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d, %s", method, path, w.Code, want, w.Body)
		}
		return w
	}
	call("GET", "/organization/profile", "", "", 401)
	call("POST", "/auth/login", `{"username":"test-admin","password":"wrong"}`, "", 401)
	login := func() *http.Cookie {
		return call("POST", "/auth/login", `{"username":"test-admin","password":"`+password+`"}`, "https://cryol.test", 200).Result().Cookies()[0]
	}
	cookie = login()
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected cookie flags: %+v", cookie)
	}
	call("GET", "/auth/me", "", "", 200)
	call("POST", "/assets", `{"name":"Test laptop"}`, "", 409)
	profile := `{"name":"Cryol Test","timezone":"UTC","currency":"USD","financialYearMonth":4,"assetPrefix":"IT"}`
	call("PUT", "/organization/profile", profile, "https://other.test", 403)
	call("PUT", "/organization/profile", profile, "", 200)
	if result := call("GET", "/organization/profile", "", "", 200); !strings.Contains(result.Body.String(), "Cryol Test") {
		t.Fatal("saved profile is missing")
	}
	var asset assetRecord
	created := call("POST", "/assets", `{"name":"Test laptop","serial":"TEST-01","manufacturer":"Example","purchaseDate":"2025-05-08"}`, "", 201)
	if err := json.Unmarshal(created.Body.Bytes(), &asset); err != nil || asset.Number != "IT-000001" || asset.ID == "" {
		t.Fatalf("asset not created: %s", created.Body)
	}
	path := "/assets/" + asset.ID
	call("GET", path, "", "", 200)
	call("PATCH", path, `{"name":"Updated laptop","serial":"TEST-01","manufacturer":"Example","purchaseDate":"2025-05-08","revision":1}`, "", 200)
	call("PATCH", path, `{"name":"Stale write","revision":1}`, "", 409)
	if result := call("GET", "/assets?page=1&q=TEST-01&manufacturer=Example&from=2025-05-01", "", "", 200); !strings.Contains(result.Body.String(), "Updated laptop") {
		t.Fatalf("saved asset missing: %s", result.Body)
	}
	call("POST", path+"/archive", `{"revision":2}`, "", 200)
	if result := call("GET", "/assets?state=active", "", "", 200); strings.Contains(result.Body.String(), "Updated laptop") {
		t.Fatalf("archived asset appears in active view: %s", result.Body)
	}
	if result := call("GET", "/dashboard/summary", "", "", 200); !strings.Contains(result.Body.String(), `"archivedAssets":1`) {
		t.Fatalf("dashboard count missing: %s", result.Body)
	}
	call("POST", path+"/restore", `{"revision":3}`, "", 200)
	if result := call("GET", path, "", "", 200); !strings.Contains(result.Body.String(), "asset.restored") {
		t.Fatalf("asset history missing: %s", result.Body)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM role_permission WHERE permission_code='assets.update'"); err != nil {
		t.Fatal(err)
	}
	call("PATCH", path, `{"name":"Unauthorized","revision":4}`, "", 403)
	testAgentInventory(t, handler, call, asset.ID)

	old := cookie
	cookie = login()
	current := cookie
	cookie = old
	call("GET", "/auth/me", "", "", 401)
	cookie = current
	if _, err := pool.Exec(ctx, "DELETE FROM role_permission WHERE permission_code='settings.manage'"); err != nil {
		t.Fatal(err)
	}
	call("PUT", "/organization/profile", profile, "", 403)
	if _, err := pool.Exec(ctx, "UPDATE user_session SET expires_at=now()"); err != nil {
		t.Fatal(err)
	}
	call("GET", "/auth/me", "", "", 401)
	cookie = login()
	call("POST", "/auth/logout", "", "", 200)
	call("GET", "/auth/me", "", "", 401)
	var events int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE event='organization.profile.updated'").Scan(&events); err != nil || events != 1 {
		t.Fatalf("expected one audited update, got %d (%v)", events, err)
	}
}

func testAgentInventory(t *testing.T, handler http.Handler, console func(string, string, string, string, int) *httptest.ResponseRecorder, assetID string) {
	t.Helper()
	var enrollment struct{ ID, Token string }
	if err := json.Unmarshal(console("POST", "/agents/enrollment-tokens", "", "", 201).Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	send := func(secure bool, path, token string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		scheme := "http"
		if secure {
			scheme = "https"
		}
		req := httptest.NewRequest("POST", scheme+"://cryol.test/api/agent/v1/"+path, strings.NewReader(string(payload)))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s: got %d want %d: %s", path, w.Code, want, w.Body)
		}
		return w
	}
	id := inventory.NewID()
	input := map[string]string{"installationId": id}
	send(false, "enroll", enrollment.Token, input, 403)
	var agent struct{ Credential string }
	if err := json.Unmarshal(send(true, "enroll", enrollment.Token, input, 201).Body.Bytes(), &agent); err != nil {
		t.Fatal(err)
	}
	send(true, "enroll", enrollment.Token, map[string]string{"installationId": inventory.NewID()}, 401)
	send(true, "heartbeat", agent.Credential, nil, 200)
	snapshot := inventory.Snapshot{Version: 1, RunID: inventory.NewID(), ScannedAt: time.Now().UTC().Add(-time.Minute), AgentVersion: "test",
		Hostname: "endpoint-one", OSFamily: "linux", OSName: "Test Linux", Serial: "SERIAL-123", Architecture: "amd64", LogicalCPUs: 4, SoftwareStatus: "complete",
		Software: []inventory.Software{{ID: "cryol-test", Name: "Cryol test package", Source: "dpkg"}}}
	send(true, "inventory", agent.Credential, snapshot, 200)
	send(true, "inventory", agent.Credential, snapshot, 200)
	firstRun := snapshot.RunID
	snapshot.Hostname = "different"
	send(true, "inventory", agent.Credential, snapshot, 409)
	snapshot.RunID, snapshot.ScannedAt, snapshot.SoftwareStatus, snapshot.Software = inventory.NewID(), time.Now().UTC(), "unavailable", nil
	send(true, "inventory", agent.Credential, snapshot, 200)
	var history struct {
		Items []struct {
			RunID, SoftwareStatus string
			ScannedAt, ReceivedAt time.Time
		}
		HasNext bool
	}
	path := "/agents/" + id + "/inventory-history"
	if err := json.Unmarshal(console("GET", path, "", "", 200).Body.Bytes(), &history); err != nil || len(history.Items) != 2 || history.HasNext {
		t.Fatalf("unexpected inventory history: %+v (%v)", history, err)
	}
	if latest := history.Items[0]; latest.RunID != snapshot.RunID || latest.SoftwareStatus != "unavailable" || latest.ReceivedAt.IsZero() || latest.ScannedAt.IsZero() || history.Items[1].RunID != firstRun {
		t.Fatalf("unexpected history order or scan metadata: %+v", history)
	}
	console("GET", path+"?page=0", "", "", 400)
	console("GET", "/agents/"+inventory.NewID()+"/inventory-history", "", "", 404)
	hardware := console("GET", "/agents/"+id+"/inventory?part=hardware", "", "", 200).Body.String()
	if strings.Contains(hardware, "cryol-test") || !strings.Contains(hardware, "different") {
		t.Fatalf("unexpected hardware report: %s", hardware)
	}
	software := console("GET", "/agents/"+id+"/inventory?part=software", "", "", 200).Body.String()
	if !strings.Contains(software, "Cryol test package") {
		t.Fatalf("last complete software missing: %s", software)
	}
	console("PUT", "/agents/"+id+"/asset", `{"assetId":"`+assetID+`"}`, "", 200)
	var list struct {
		Items []struct {
			ID, DeviceID, Hostname, OSName, Serial, Status, AssetID string
			LastSeen                                                time.Time
		}
	}
	if err := json.Unmarshal(console("GET", "/agents?page=1", "", "", 200).Body.Bytes(), &list); err != nil || len(list.Items) != 1 {
		t.Fatalf("unexpected agent list: %+v (%v)", list, err)
	}
	item := list.Items[0]
	if item.ID != id || item.DeviceID == "" || item.Hostname != "different" || item.OSName != "Test Linux" || item.Serial != "SERIAL-123" || item.Status != "online" || item.AssetID != assetID || item.LastSeen.IsZero() {
		t.Fatalf("unexpected agent table row: %+v", item)
	}
	console("PUT", "/agents/"+id+"/asset", `{"assetId":null}`, "", 200)
	console("POST", "/agents/"+id+"/revoke", "", "", 200)
	send(true, "heartbeat", agent.Credential, nil, 401)
	send(true, "inventory", agent.Credential, snapshot, 401)
}
