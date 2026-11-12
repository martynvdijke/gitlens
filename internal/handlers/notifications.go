package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitlens/ent"
	"gitlens/ent/devicetoken"
	"gitlens/internal/secrets"

	"github.com/gin-gonic/gin"
)

// NotificationHandler serves the public FCM config and the per-user
// notification preferences/device endpoints.
type NotificationHandler struct {
	client *ent.Client
}

func NewNotificationHandler(client *ent.Client) *NotificationHandler {
	return &NotificationHandler{client: client}
}

// Config is the public Firebase client configuration. It contains no
// credentials — only the values a browser/client needs to connect to FCM.
func (h *NotificationHandler) Config(c *gin.Context) {
	ctx := c.Request.Context()

	resp := gin.H{
		"enabled":        false,
		"project_id":     "",
		"api_key":        "",
		"app_id":         "",
		"sender_id":      "",
		"storage_bucket": "",
	}
	cfg, err := h.client.AdminConfig.Get(ctx, 1)
	if err != nil && !ent.IsNotFound(err) {
		log.Printf("notifications: error loading config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load notification configuration"})
		return
	}
	if cfg != nil {
		resp = gin.H{
			"enabled":        cfg.NotificationsEnabled && cfg.FcmProjectID != "" && cfg.FcmServiceAccountJSON != "",
			"project_id":     cfg.FcmProjectID,
			"api_key":        cfg.FcmAPIKey,
			"app_id":         cfg.FcmAppID,
			"sender_id":      cfg.FcmSenderID,
			"storage_bucket": cfg.FcmStorageBucket,
		}
	}
	c.JSON(http.StatusOK, resp)
}

// prefsResponse is the JSON shape for GET/POST /api/v1/notifications/prefs.
type prefsResponse struct {
	NotificationsEnabled  bool `json:"notifications_enabled"`
	NotifyWeb             bool `json:"notify_web"`
	NotifyAndroid         bool `json:"notify_android"`
	NotifyRelease         bool `json:"notify_release"`
	NotifyPrMerge         bool `json:"notify_pr_merge"`
	NotifyWorkflowFailure bool `json:"notify_workflow_failure"`
}

func prefsFor(u *ent.User) prefsResponse {
	return prefsResponse{
		NotificationsEnabled:  u.NotificationsEnabled,
		NotifyWeb:             u.NotifyWeb,
		NotifyAndroid:         u.NotifyAndroid,
		NotifyRelease:         u.NotifyRelease,
		NotifyPrMerge:         u.NotifyPrMerge,
		NotifyWorkflowFailure: u.NotifyWorkflowFailure,
	}
}

// GetPrefs returns the current user's notification preferences as JSON.
func (h *NotificationHandler) GetPrefs(c *gin.Context) {
	u, err := h.currentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, prefsFor(u))
}

// prefsUpdate uses pointers so a partial JSON body only changes the provided
// fields.
type prefsUpdate struct {
	NotificationsEnabled  *bool `json:"notifications_enabled"`
	NotifyWeb             *bool `json:"notify_web"`
	NotifyAndroid         *bool `json:"notify_android"`
	NotifyRelease         *bool `json:"notify_release"`
	NotifyPrMerge         *bool `json:"notify_pr_merge"`
	NotifyWorkflowFailure *bool `json:"notify_workflow_failure"`
}

// UpdatePrefs applies a JSON preference update.
func (h *NotificationHandler) UpdatePrefs(c *gin.Context) {
	u, err := h.currentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req prefsUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	upd := h.client.User.UpdateOneID(u.ID)
	if req.NotificationsEnabled != nil {
		upd = upd.SetNotificationsEnabled(*req.NotificationsEnabled)
	}
	if req.NotifyWeb != nil {
		upd = upd.SetNotifyWeb(*req.NotifyWeb)
	}
	if req.NotifyAndroid != nil {
		upd = upd.SetNotifyAndroid(*req.NotifyAndroid)
	}
	if req.NotifyRelease != nil {
		upd = upd.SetNotifyRelease(*req.NotifyRelease)
	}
	if req.NotifyPrMerge != nil {
		upd = upd.SetNotifyPrMerge(*req.NotifyPrMerge)
	}
	if req.NotifyWorkflowFailure != nil {
		upd = upd.SetNotifyWorkflowFailure(*req.NotifyWorkflowFailure)
	}

	updated, err := upd.Save(c.Request.Context())
	if err != nil {
		log.Printf("notifications: error saving prefs for user %d: %v", u.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save preferences"})
		return
	}
	c.JSON(http.StatusOK, prefsFor(updated))
}

// UpdatePrefsForm applies the settings-page checkbox form and renders the
// small HTMX status partial.
func (h *NotificationHandler) UpdatePrefsForm(c *gin.Context) {
	u, err := h.currentUser(c)
	if err != nil {
		c.String(http.StatusUnauthorized, "Not signed in")
		return
	}
	updated, err := h.client.User.UpdateOneID(u.ID).
		SetNotificationsEnabled(checkedForm(c, "notifications_enabled")).
		SetNotifyWeb(checkedForm(c, "notify_web")).
		SetNotifyAndroid(checkedForm(c, "notify_android")).
		SetNotifyRelease(checkedForm(c, "notify_release")).
		SetNotifyPrMerge(checkedForm(c, "notify_pr_merge")).
		SetNotifyWorkflowFailure(checkedForm(c, "notify_workflow_failure")).
		Save(c.Request.Context())
	if err != nil {
		log.Printf("notifications: error saving prefs form for user %d: %v", u.ID, err)
		c.String(http.StatusInternalServerError, "Failed to save notification settings")
		return
	}
	c.HTML(http.StatusOK, "notification_settings_status", gin.H{"User": updated, "Saved": true})
}

// deviceRequest is shared by the JSON and form device-registration paths.
type deviceRequest struct {
	Token    string `json:"token" form:"token"`
	Platform string `json:"platform" form:"platform"`
}

// bindDeviceRequest accepts JSON or form-encoded bodies. DELETE bodies are not
// parsed by net/http's ParseForm, so form bodies are read directly; query
// parameters are accepted as a fallback.
func bindDeviceRequest(c *gin.Context, req *deviceRequest) error {
	if strings.HasPrefix(c.ContentType(), "application/json") {
		return c.ShouldBindJSON(req)
	}
	if c.Request.Body != nil {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return err
		}
		if len(body) > 0 {
			vals, err := url.ParseQuery(string(body))
			if err != nil {
				return err
			}
			req.Token = vals.Get("token")
			req.Platform = vals.Get("platform")
		}
	}
	if req.Token == "" {
		req.Token = c.Query("token")
	}
	if req.Platform == "" {
		req.Platform = c.Query("platform")
	}
	return nil
}

// RegisterDevice upserts an FCM registration token for the current user. A
// token that already exists for another user is reassigned; the same user's
// token only refreshes last_seen_at.
func (h *NotificationHandler) RegisterDevice(c *gin.Context) {
	u, err := h.currentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req deviceRequest
	if err := bindDeviceRequest(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	req.Platform = strings.TrimSpace(req.Platform)
	if req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}
	if req.Platform != "android" && req.Platform != "web" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "platform must be android or web"})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.client.DeviceToken.Query().
		Where(devicetoken.TokenEQ(req.Token)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		log.Printf("notifications: error looking up device token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register device"})
		return
	}

	if existing == nil || ent.IsNotFound(err) {
		_, err = h.client.DeviceToken.Create().
			SetUserID(u.ID).
			SetToken(req.Token).
			SetPlatform(devicetoken.Platform(req.Platform)).
			SetLastSeenAt(time.Now()).
			Save(ctx)
	} else {
		upd := h.client.DeviceToken.UpdateOneID(existing.ID).
			SetPlatform(devicetoken.Platform(req.Platform)).
			SetLastSeenAt(time.Now())
		if existing.UserID != u.ID {
			upd = upd.SetUserID(u.ID)
		}
		_, err = upd.Save(ctx)
	}
	if err != nil {
		log.Printf("notifications: error saving device token for user %d: %v", u.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register device"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UnregisterDevice removes a token owned by the current user.
func (h *NotificationHandler) UnregisterDevice(c *gin.Context) {
	u, err := h.currentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req deviceRequest
	if err := bindDeviceRequest(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}
	_, err = h.client.DeviceToken.Delete().
		Where(
			devicetoken.TokenEQ(req.Token),
			devicetoken.UserIDEQ(u.ID),
		).
		Exec(c.Request.Context())
	if err != nil {
		log.Printf("notifications: error deleting device token for user %d: %v", u.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unregister device"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NotificationHandler) currentUser(c *gin.Context) (*ent.User, error) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		return nil, errors.New("no user in context")
	}
	return h.client.User.Get(c.Request.Context(), int(userID))
}

// checkedForm reports whether a checkbox was submitted. Browsers send "on"
// when no value attribute is present; true/1 are accepted for API clients.
func checkedForm(c *gin.Context, name string) bool {
	switch strings.ToLower(strings.TrimSpace(c.PostForm(name))) {
	case "on", "true", "1", "yes":
		return true
	}
	return false
}

// ---- admin notification configuration ----

// UpdateNotificationConfig saves the global notification switches and FCM
// credentials. A blank service-account field keeps the stored value.
func (h *AdminHandler) UpdateNotificationConfig(c *gin.Context) {
	ctx := c.Request.Context()

	notificationsEnabled := checkedForm(c, "notifications_enabled")
	notifyRelease := checkedForm(c, "notify_release")
	notifyPRMerge := checkedForm(c, "notify_pr_merge")
	notifyWorkflowFailure := checkedForm(c, "notify_workflow_failure")

	projectID := strings.TrimSpace(c.PostForm("fcm_project_id"))
	serviceAccount := strings.TrimSpace(c.PostForm("fcm_service_account_json"))
	apiKey := strings.TrimSpace(c.PostForm("fcm_api_key"))
	appID := strings.TrimSpace(c.PostForm("fcm_app_id"))
	senderID := strings.TrimSpace(c.PostForm("fcm_sender_id"))
	storageBucket := strings.TrimSpace(c.PostForm("fcm_storage_bucket"))

	if serviceAccount != "" {
		sealed, err := secrets.Encrypt(serviceAccount)
		if err != nil {
			log.Printf("admin: failed to encrypt FCM service account: %v", err)
			c.String(http.StatusInternalServerError, "Failed to save notification configuration")
			return
		}
		serviceAccount = sealed
	}

	_, err := h.client.AdminConfig.Get(ctx, 1)
	if ent.IsNotFound(err) {
		_, err = h.client.AdminConfig.Create().
			SetID(1).
			SetNotificationsEnabled(notificationsEnabled).
			SetNotifyRelease(notifyRelease).
			SetNotifyPrMerge(notifyPRMerge).
			SetNotifyWorkflowFailure(notifyWorkflowFailure).
			SetFcmProjectID(projectID).
			SetFcmServiceAccountJSON(serviceAccount).
			SetFcmAPIKey(apiKey).
			SetFcmAppID(appID).
			SetFcmSenderID(senderID).
			SetFcmStorageBucket(storageBucket).
			Save(ctx)
	} else if err == nil {
		u := h.client.AdminConfig.UpdateOneID(1).
			SetNotificationsEnabled(notificationsEnabled).
			SetNotifyRelease(notifyRelease).
			SetNotifyPrMerge(notifyPRMerge).
			SetNotifyWorkflowFailure(notifyWorkflowFailure).
			SetFcmProjectID(projectID).
			SetFcmAPIKey(apiKey).
			SetFcmAppID(appID).
			SetFcmSenderID(senderID).
			SetFcmStorageBucket(storageBucket)
		if serviceAccount != "" {
			u = u.SetFcmServiceAccountJSON(serviceAccount)
		}
		_, err = u.Save(ctx)
	}
	if err != nil {
		log.Printf("admin: error saving notification config: %v", err)
		c.String(http.StatusInternalServerError, "Failed to save notification configuration")
		return
	}

	h.renderNotificationsForm(c)
}

// TestNotificationFCM validates the saved service-account credentials.
func (h *AdminHandler) TestNotificationFCM(c *gin.Context) {
	if h.fcmTest == nil {
		c.HTML(http.StatusServiceUnavailable, "fcm_test_status", gin.H{
			"OK":      false,
			"Message": "Notifications are not configured. Save a project ID and service account first.",
		})
		return
	}
	if err := h.fcmTest(c.Request.Context()); err != nil {
		if errors.Is(err, ErrFCMNotConfigured) {
			c.HTML(http.StatusServiceUnavailable, "fcm_test_status", gin.H{
				"OK":      false,
				"Message": "Notifications are not configured. Save a project ID and service account first.",
			})
			return
		}
		log.Printf("admin: FCM credential test failed")
		c.HTML(http.StatusBadGateway, "fcm_test_status", gin.H{
			"OK":      false,
			"Message": "FCM credentials could not be validated. Check the project ID and service account.",
		})
		return
	}
	c.HTML(http.StatusOK, "fcm_test_status", gin.H{
		"OK":      true,
		"Message": "FCM credentials are valid.",
	})
}

// ErrFCMNotConfigured lets the runtime distinguish missing credentials from
// a configured client that failed.
var ErrFCMNotConfigured = errors.New("fcm not configured")

func (h *AdminHandler) renderNotificationsForm(c *gin.Context) {
	ctx := c.Request.Context()
	cfg, err := h.client.AdminConfig.Get(ctx, 1)
	if err != nil && !ent.IsNotFound(err) {
		log.Printf("admin: error loading config for notifications form: %v", err)
	}
	c.HTML(http.StatusOK, "admin_notifications_form", gin.H{"Config": cfg})
}
