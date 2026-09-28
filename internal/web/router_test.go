package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"pxe/internal/config"
	"pxe/internal/observability"
	"pxe/internal/storage"
	"strings"
	"sync"
	"testing"
)

type testBackend struct {
	store *storage.Store
	hub   *observability.Hub
}

func (b testBackend) Storage() *storage.Store             { return b.store }
func (b testBackend) EventHub() *observability.Hub        { return b.hub }
func (b testBackend) BootConfig() config.BootConfig       { return config.Default() }
func (b testBackend) Status() any                         { return map[string]string{"state": "test"} }
func (b testBackend) StartServices(context.Context) error { return nil }
func (b testBackend) StopServices(context.Context) error  { return nil }
func testRouter(t *testing.T) (http.Handler, *storage.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.Open(context.Background(), filepath.Join(dir, "pxe.db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewRouter(testBackend{s, observability.NewHub(s)}), s
}
func request(r http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	q := httptest.NewRequest(method, path, strings.NewReader(body))
	q.Header.Set("Content-Type", "application/json")
	if token != "" {
		q.AddCookie(&http.Cookie{Name: "pxe_session", Value: token})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	return w
}
func loginToken(t *testing.T, r http.Handler, user, pass string) string {
	t.Helper()
	w := request(r, "POST", "/api/v1/auth/login", fmt.Sprintf(`{"Username":%q,"Password":%q}`, user, pass), "")
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	return w.Result().Cookies()[0].Value
}
func setupAdmin(t *testing.T, r http.Handler) string {
	t.Helper()
	w := request(r, "POST", "/api/v1/setup", `{"Username":"admin","Password":"password123"}`, "")
	if w.Code != 200 {
		t.Fatalf("setup: %s", w.Body)
	}
	return loginToken(t, r, "admin", "password123")
}
func TestSessionRevocationExpirationAndDeletion(t *testing.T) {
	r, s := testRouter(t)
	token := setupAdmin(t, r)
	if w := request(r, "POST", "/api/v1/auth/logout", "", token); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := request(r, "GET", "/api/v1/status", "", token); w.Code != 401 {
		t.Fatal("logged out token accepted")
	}
	token = loginToken(t, r, "admin", "password123")
	if w := request(r, "POST", "/api/v1/users/1/password", `{"password":"newpassword123","current_password":"password123"}`, token); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := request(r, "GET", "/api/v1/status", "", token); w.Code != 401 {
		t.Fatal("password reset did not revoke")
	}
	token = loginToken(t, r, "admin", "newpassword123")
	if w := request(r, "POST", "/api/v1/users", `{"Username":"second","Password":"password123"}`, token); w.Code != 200 {
		t.Fatal(w.Body)
	}
	other := loginToken(t, r, "second", "password123")
	s.RawDB().SetMaxIdleConns(0) // force fresh SQLite connections before deletion
	if w := request(r, "DELETE", "/api/v1/users/2", "", token); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := request(r, "POST", "/api/v1/users", `{"Username":"replacement","Password":"password123"}`, token); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := request(r, "GET", "/api/v1/status", "", other); w.Code != 401 {
		t.Fatal("deleted user token accepted")
	}
	if _, err := s.RawDB().Exec(`UPDATE sessions SET expires=0`); err != nil {
		t.Fatal(err)
	}
	if w := request(r, "GET", "/api/v1/status", "", token); w.Code != 401 {
		t.Fatal("expired token accepted")
	}
}
func TestSetupAtomicAndRemovedRoutesAbsent(t *testing.T) {
	r, s := testRouter(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request(r, "POST", "/api/v1/setup", fmt.Sprintf(`{"Username":"admin%d","Password":"password123"}`, i), "")
		}(i)
	}
	wg.Wait()
	var count int
	if err := s.RawDB().QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("setup race: %d %v", count, err)
	}
	for _, path := range []string{"/menus", "/actions", "/api/v1/menus", "/api/v1/actions", "/api/v1/files/torrent", "/dynamic.ipxe?myip=1.2.3.4&mymac=00:11:22:33:44:55"} {
		if w := request(r, "GET", path, "", ""); w.Code != 404 {
			t.Errorf("removed route %s: %d", path, w.Code)
		}
	}
	var tables []string
	rows, err := s.RawDB().Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, n)
	}
	for _, n := range tables {
		if n == "boot_menus" || n == "boot_menu_items" || n == "client_actions" {
			t.Errorf("retired table %s", n)
		}
	}
}
func TestConfigFailureDoesNotBypassAuth(t *testing.T) {
	r, s := testRouter(t)
	setupAdmin(t, r)
	if _, err := s.RawDB().Exec(`UPDATE settings SET value='broken'`); err != nil {
		t.Fatal(err)
	}
	if w := request(r, "GET", "/api/v1/config", "", ""); w.Code != 401 {
		t.Fatalf("auth bypass: %d", w.Code)
	}
}
func TestClientAndFileCRUD(t *testing.T) {
	r, s := testRouter(t)
	token := setupAdmin(t, r)
	w := request(r, "POST", "/api/v1/clients", `{"name":"PC","ip":"192.168.1.20","mac":"02:00:00:00:00:01"}`, token)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	w = request(r, "PUT", "/api/v1/clients/1", `{"name":"PC2","ip":"192.168.1.21","mac":"02:00:00:00:00:01"}`, token)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	cfg, err := s.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(cfg.HTTPBoot.Root, 0755); err != nil {
		t.Fatal(err)
	}
	w = request(r, "PUT", "/api/v1/files/content", `{"root":"http","path":"boot.ipxe","content":"#!ipxe\nexit\n"}`, token)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	w = request(r, "GET", "/api/v1/files/content?root=http&path=boot.ipxe", "", token)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	w = request(r, "PUT", "/api/v1/files/content", `{"root":"http","path":"../outside.txt","content":"bad"}`, token)
	if w.Code == 200 {
		t.Fatal("path escaped")
	}
	w = request(r, "GET", "/api/v1/config", "", token)
	var body map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	cfgMap := body["data"].(map[string]any)
	if _, ok := cfgMap["torrent"]; ok {
		t.Fatal("retired config")
	}
}

func TestFirmwareCatalogAndSelection(t *testing.T) {
	r, _ := testRouter(t)
	if w := request(r, "GET", "/api/v1/firmware", "", ""); w.Code != 401 {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	token := setupAdmin(t, r)
	w := request(r, "GET", "/api/v1/firmware", "", token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "ipxe-arm64.efi") || !strings.Contains(w.Body.String(), "netboot.xyz.efi") {
		t.Fatalf("catalog: %d %s", w.Code, w.Body)
	}
	for _, body := range []string{`{"source":"project","files":["../escape"]}`, `{"source":"unknown","files":["undionly.kpxe"]}`, `{"source":"project","files":[]}`} {
		if w := request(r, "POST", "/api/v1/firmware/download", body, token); w.Code != 400 {
			t.Fatalf("invalid selection: %d %s", w.Code, w.Body)
		}
	}
}

func TestInvalidUpdateIDCannotCreateClient(t *testing.T) {
	r, s := testRouter(t)
	token := setupAdmin(t, r)
	for _, id := range []string{"bad", "0", "-1", "999999999999999999999999"} {
		w := request(r, "PUT", "/api/v1/clients/"+id, `{"name":"must not create"}`, token)
		if w.Code != 400 {
			t.Fatalf("%s: %d", id, w.Code)
		}
	}
	rows, err := s.ListClients(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
func TestHistoryAndLiveEventsShareIdentity(t *testing.T) {
	r, s := testRouter(t)
	token := setupAdmin(t, r)
	rows, err := s.RecentEvents(context.Background(), 100)
	if err != nil || len(rows) == 0 {
		t.Fatal("events not persisted", rows, err)
	}
	w := request(r, "GET", "/api/v1/logs", "", token)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"ts"`) {
		t.Fatal(w.Body)
	}
	if _, err = s.RawDB().Exec(`DROP TABLE events`); err != nil {
		t.Fatal(err)
	}
	if w = request(r, "GET", "/api/v1/logs", "", token); w.Code != 500 {
		t.Fatal("database failure hidden", w.Code)
	}
}

func TestRetiredWriteAndReportEndpointsAreAbsent(t *testing.T) {
	r, _ := testRouter(t)
	token := setupAdmin(t, r)
	for _, path := range []string{"/api/v1/clients/report", "/api/v1/config/validate", "/api/v1/services/restart"} {
		if w := request(r, "POST", path, `{}`, token); w.Code != 404 {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
}

func TestOwnPasswordRequiresCurrentPassword(t *testing.T) {
	r, _ := testRouter(t)
	token := setupAdmin(t, r)
	for _, body := range []string{
		`{"password":"replacement123"}`,
		`{"password":"replacement123","current_password":"incorrect"}`,
		`{"password":"replacement123","current":false,"reset":true,"actor":2}`,
	} {
		if w := request(r, "POST", "/api/v1/users/1/password", body, token); w.Code != 400 {
			t.Fatalf("bypassed current password: %d %s", w.Code, w.Body)
		}
	}
	if w := request(r, "GET", "/api/v1/status", "", token); w.Code != 200 {
		t.Fatal("failed change revoked session")
	}
	second := loginToken(t, r, "admin", "password123")
	if w := request(r, "POST", "/api/v1/users/1/password", `{"password":"replacement123","current_password":"password123"}`, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"reauthenticate":true`) {
		t.Fatal(w.Code, w.Body)
	}
	for _, session := range []string{token, second} {
		if w := request(r, "GET", "/api/v1/status", "", session); w.Code != 401 {
			t.Fatal("old session remains valid")
		}
	}
	loginToken(t, r, "admin", "replacement123")
	if w := request(r, "POST", "/api/v1/auth/login", `{"username":"admin","password":"password123"}`, ""); w.Code != 401 {
		t.Fatal("old password remains valid")
	}
}

func TestAdminResetAndCurrentIdentity(t *testing.T) {
	r, s := testRouter(t)
	admin := setupAdmin(t, r)
	if w := request(r, "POST", "/api/v1/users", `{"username":"second","password":"password123"}`, admin); w.Code != 200 {
		t.Fatal(w.Body)
	}
	target := loginToken(t, r, "second", "password123")
	other := loginToken(t, r, "second", "password123")
	for _, session := range []struct {
		token string
		id    int64
	}{{admin, 1}, {target, 2}} {
		w := request(r, "GET", "/api/v1/users", "", session.token)
		var body struct {
			Data []storage.User `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, user := range body.Data {
			if user.Current != (user.ID == session.id) {
				t.Fatal("wrong current identity", body)
			}
		}
	}
	if w := request(r, "POST", "/api/v1/users/2/password", `{"password":"reset-password123"}`, admin); w.Code != 200 || !strings.Contains(w.Body.String(), `"reauthenticate":false`) {
		t.Fatal(w.Code, w.Body)
	}
	for _, token := range []string{target, other} {
		if w := request(r, "GET", "/api/v1/status", "", token); w.Code != 401 {
			t.Fatal("target session not revoked")
		}
	}
	if w := request(r, "GET", "/api/v1/status", "", admin); w.Code != 200 {
		t.Fatal("actor session revoked")
	}
	target = loginToken(t, r, "second", "reset-password123")
	if _, err := s.RawDB().Exec(`UPDATE users SET role='viewer' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if w := request(r, "POST", "/api/v1/users/1/password", `{"password":"forbidden123"}`, target); w.Code != 403 {
		t.Fatal("non-admin reset accepted", w.Code)
	}
	loginToken(t, r, "admin", "password123")
}

func TestStaticPagesAndMissingAssets(t *testing.T) {
	r, _ := testRouter(t)
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/", 200}, {"/files", 200}, {"/assets/missing.js", 404}, {"/unknown-page", 404},
	} {
		w := request(r, "GET", tc.path, "", "")
		if w.Code != tc.status {
			t.Fatalf("%s: got %d, want %d", tc.path, w.Code, tc.status)
		}
	}
}
