package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gitlens/ent"
	"gitlens/ent/devicetoken"
	"gitlens/ent/enttest"
	"gitlens/internal/secrets"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

var notificationTestTmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"printf": fmt.Sprintf,
}).Parse(`
{{define "notification_settings_status"}}<span>saved={{.Saved}}</span>{{end}}
{{define "admin_notifications_form"}}<div>fcm_form project={{if .Config}}{{.Config.FcmProjectID}}{{end}}</div>{{end}}
{{define "fcm_test_status"}}<span>{{if .OK}}ok{{else}}fail{{end}}:{{.Message}}</span>{{end}}
`))

func newNotificationTest(t *testing.T) (*ent.Client, *ent.User, *NotificationHandler) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	u, err := client.User.Create().
		SetGithubID(100).
		SetLogin("user").
		SetAccessToken("tok").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return client, u, NewNotificationHandler(client)
}

func notificationEngine(h *NotificationHandler, userID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(notificationTestTmpl)
	engine.Use(func(c *gin.Context) {
		if userID != 0 {
			c.Set("user_id", userID)
		}
		c.Next()
	})
	engine.GET("/api/v1/notifications/config", h.Config)
	engine.GET("/api/v1/notifications/prefs", h.GetPrefs)
	engine.POST("/api/v1/notifications/prefs", h.UpdatePrefs)
	engine.POST("/api/v1/notifications/device", h.RegisterDevice)
	engine.DELETE("/api/v1/notifications/device", h.UnregisterDevice)
	engine.POST("/settings/notifications", h.UpdatePrefsForm)
	return engine
}

func notificationJSON(engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func notificationForm(engine *gin.Engine, method, path string, vals url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestPublicConfigHidesSecrets(t *testing.T) {
	client, _, h := newNotificationTest(t)
	ctx := context.Background()
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetNotificationsEnabled(true).
		SetFcmProjectID("my-project").
		SetFcmServiceAccountJSON("super-secret-service-account").
		SetFcmAPIKey("api-key").
		SetFcmAppID("app-id").
		SetFcmSenderID("sender-id").
		SetFcmStorageBucket("bucket").
		Save(ctx)
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	engine := notificationEngine(h, 0)
	w := notificationJSON(engine, http.MethodGet, "/api/v1/notifications/config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "super-secret-service-account") {
		t.Fatal("public config leaked the service account")
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["enabled"] != true {
		t.Errorf("enabled = %v", resp["enabled"])
	}
	if resp["project_id"] != "my-project" {
		t.Errorf("project_id = %v", resp["project_id"])
	}
	if resp["api_key"] != "api-key" {
		t.Errorf("api_key = %v", resp["api_key"])
	}
}

func TestPrefsJSONAndForm(t *testing.T) {
	client, u, h := newNotificationTest(t)
	engine := notificationEngine(h, int64(u.ID))

	// Defaults are all on.
	w := notificationJSON(engine, http.MethodGet, "/api/v1/notifications/prefs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET prefs status = %d", w.Code)
	}
	var prefs map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &prefs); err != nil {
		t.Fatalf("decode prefs: %v", err)
	}
	if !prefs["notify_web"] || !prefs["notifications_enabled"] {
		t.Fatalf("unexpected defaults: %v", prefs)
	}

	// Partial JSON update.
	w = notificationJSON(engine, http.MethodPost, "/api/v1/notifications/prefs", `{"notify_web":false,"notify_release":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST prefs status = %d body=%s", w.Code, w.Body.String())
	}
	got, err := client.User.Get(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NotifyWeb {
		t.Error("notify_web should be false")
	}
	if got.NotifyRelease {
		t.Error("notify_release should be false")
	}
	if !got.NotifyAndroid {
		t.Error("notify_android should be unchanged (true)")
	}

	// Form update: only checked boxes are true.
	w = notificationForm(engine, http.MethodPost, "/settings/notifications", url.Values{
		"notifications_enabled": {"on"},
		"notify_android":        {"on"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("form status = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "saved=true") {
		t.Fatalf("expected saved partial, got %s", w.Body.String())
	}
	got, err = client.User.Get(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.NotificationsEnabled || !got.NotifyAndroid {
		t.Error("checked boxes should be enabled")
	}
	if got.NotifyWeb || got.NotifyRelease || got.NotifyPrMerge || got.NotifyWorkflowFailure {
		t.Error("unchecked boxes should be disabled")
	}
}

func TestDeviceRegisterReassignUnregister(t *testing.T) {
	client, u, h := newNotificationTest(t)
	other, err := client.User.Create().
		SetGithubID(101).
		SetLogin("other").
		SetAccessToken("tok").
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	engine := notificationEngine(h, int64(u.ID))
	w := notificationJSON(engine, http.MethodPost, "/api/v1/notifications/device", `{"token":"device-1","platform":"android"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("register status = %d body=%s", w.Code, w.Body.String())
	}
	dt, err := client.DeviceToken.Query().Where(devicetoken.TokenEQ("device-1")).Only(context.Background())
	if err != nil {
		t.Fatalf("query token: %v", err)
	}
	if dt.UserID != u.ID || dt.Platform != devicetoken.PlatformAndroid {
		t.Fatalf("unexpected row: %+v", dt)
	}

	// Same token, same user, different platform: update in place.
	w = notificationJSON(engine, http.MethodPost, "/api/v1/notifications/device", `{"token":"device-1","platform":"web"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d", w.Code)
	}
	n, _ := client.DeviceToken.Query().Count(context.Background())
	if n != 1 {
		t.Fatalf("expected 1 token row, got %d", n)
	}
	dt, _ = client.DeviceToken.Query().Where(devicetoken.TokenEQ("device-1")).Only(context.Background())
	if dt.UserID != u.ID || dt.Platform != devicetoken.PlatformWeb {
		t.Fatalf("row not updated: %+v", dt)
	}

	// Same token for another user: reassign.
	otherEngine := notificationEngine(h, int64(other.ID))
	w = notificationJSON(otherEngine, http.MethodPost, "/api/v1/notifications/device", `{"token":"device-1","platform":"android"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("reassign status = %d", w.Code)
	}
	dt, _ = client.DeviceToken.Query().Where(devicetoken.TokenEQ("device-1")).Only(context.Background())
	if dt.UserID != other.ID {
		t.Fatalf("token not reassigned: %+v", dt)
	}

	// Unregister as the original user must not delete another user's token.
	w = notificationForm(engine, http.MethodDelete, "/api/v1/notifications/device", url.Values{"token": {"device-1"}})
	if w.Code != http.StatusOK {
		t.Fatalf("unregister status = %d", w.Code)
	}
	n, _ = client.DeviceToken.Query().Count(context.Background())
	if n != 1 {
		t.Fatalf("token owned by another user should remain, count=%d", n)
	}

	// Correct owner can unregister.
	w = notificationForm(otherEngine, http.MethodDelete, "/api/v1/notifications/device", url.Values{"token": {"device-1"}})
	if w.Code != http.StatusOK {
		t.Fatalf("owner unregister status = %d", w.Code)
	}
	n, _ = client.DeviceToken.Query().Count(context.Background())
	if n != 0 {
		t.Fatalf("expected token removed, count=%d", n)
	}
}

func TestRegisterDeviceValidation(t *testing.T) {
	_, u, h := newNotificationTest(t)
	engine := notificationEngine(h, int64(u.ID))

	for _, body := range []string{
		`{"token":"","platform":"android"}`,
		`{"token":"tok","platform":"ios"}`,
		`{"token":"tok"}`,
	} {
		w := notificationJSON(engine, http.MethodPost, "/api/v1/notifications/device", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
	}
}

func TestAdminNotificationConfig(t *testing.T) {
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "test-key")
	client, _, _ := newNotificationTest(t)
	h := NewAdminHandler(client, nil)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(notificationTestTmpl)
	engine.POST("/admin/notifications", h.UpdateNotificationConfig)

	// First save with a service account: stored encrypted.
	w := notificationForm(engine, http.MethodPost, "/admin/notifications", url.Values{
		"notifications_enabled":    {"on"},
		"notify_release":           {"on"},
		"fcm_project_id":           {"proj-1"},
		"fcm_service_account_json": {`{"client_email":"a@b"}`},
		"fcm_api_key":              {"key"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("save status = %d body=%s", w.Code, w.Body.String())
	}
	cfg, err := client.AdminConfig.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.FcmServiceAccountJSON, "enc:v1:") {
		t.Fatalf("service account not encrypted: %q", cfg.FcmServiceAccountJSON)
	}
	stored := cfg.FcmServiceAccountJSON
	if cfg.FcmProjectID != "proj-1" || cfg.FcmAPIKey != "key" {
		t.Fatalf("unexpected saved config: %+v", cfg)
	}
	if !cfg.NotificationsEnabled || !cfg.NotifyRelease {
		t.Error("checked switches should be enabled")
	}
	if cfg.NotifyPrMerge || cfg.NotifyWorkflowFailure {
		t.Error("unchecked switches should be disabled")
	}

	// Second save with a blank service account keeps the stored value.
	w = notificationForm(engine, http.MethodPost, "/admin/notifications", url.Values{
		"notifications_enabled":    {"on"},
		"notify_release":           {"on"},
		"fcm_project_id":           {"proj-2"},
		"fcm_service_account_json": {""},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("second save status = %d", w.Code)
	}
	cfg, err = client.AdminConfig.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FcmServiceAccountJSON != stored {
		t.Fatal("blank save should keep the existing service account")
	}
	if cfg.FcmProjectID != "proj-2" {
		t.Fatalf("project id not updated: %q", cfg.FcmProjectID)
	}

	// The decrypted value must round-trip.
	plain, err := secrets.Decrypt(cfg.FcmServiceAccountJSON)
	if err != nil || plain != `{"client_email":"a@b"}` {
		t.Fatalf("decrypt = %q, err=%v", plain, err)
	}
}

func TestTestNotificationFCM(t *testing.T) {
	client, _, _ := newNotificationTest(t)
	h := NewAdminHandler(client, nil)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(notificationTestTmpl)
	engine.POST("/admin/notifications/test", h.TestNotificationFCM)

	// No callback wired -> service unavailable.
	w := notificationForm(engine, http.MethodPost, "/admin/notifications/test", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}

	h.SetFCMTest(func(context.Context) error { return ErrFCMNotConfigured })
	w = notificationForm(engine, http.MethodPost, "/admin/notifications/test", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-configured status = %d, want 503", w.Code)
	}

	h.SetFCMTest(func(context.Context) error { return nil })
	w = notificationForm(engine, http.MethodPost, "/admin/notifications/test", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ok:") {
		t.Fatalf("success status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestListSettingsIncludesNotificationFlags(t *testing.T) {
	client, _, _ := newNotificationTest(t)
	h := NewAdminHandler(client, nil)
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetNotificationsEnabled(true).
		SetNotifyRelease(false).
		SetFcmProjectID("proj").
		SetFcmServiceAccountJSON("enc:v1:secret").
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	h.ListSettings(c)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["notifications_enabled"] != true {
		t.Errorf("notifications_enabled = %v", resp["notifications_enabled"])
	}
	if resp["notify_release"] != false {
		t.Errorf("notify_release = %v", resp["notify_release"])
	}
	if resp["fcm_configured"] != true {
		t.Errorf("fcm_configured = %v", resp["fcm_configured"])
	}
	if strings.Contains(w.Body.String(), "enc:v1:secret") {
		t.Fatal("ListSettings leaked the service account")
	}
}
