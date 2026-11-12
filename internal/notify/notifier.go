// Package notify delivers activity-feed notifications to users over the
// browser WebSocket channel and Android FCM push. Delivery is best-effort: a
// failure never propagates back to the sync loop.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gitlens/ent"
	"gitlens/ent/devicetoken"
	"gitlens/ent/event"
	"gitlens/ent/user"
	"gitlens/internal/fcm"
	"gitlens/internal/secrets"
	"gitlens/internal/ws"
)

// Notifier turns newly created Event rows into user notifications.
type Notifier struct {
	client *ent.Client
	hub    *ws.Hub

	mu      sync.Mutex
	clients map[string]*fcm.Client
}

// New builds a notifier. hub may be nil (web delivery is then skipped).
func New(client *ent.Client, hub *ws.Hub) *Notifier {
	return &Notifier{
		client:  client,
		hub:     hub,
		clients: make(map[string]*fcm.Client),
	}
}

// eventTypeEnabled reports whether the admin default and the user preference
// both allow the given event type.
func eventTypeEnabled(cfg *ent.AdminConfig, u *ent.User, t event.Type) bool {
	switch t {
	case event.TypeRelease:
		return (cfg == nil || cfg.NotifyRelease) && u.NotifyRelease
	case event.TypePrMerge:
		return (cfg == nil || cfg.NotifyPrMerge) && u.NotifyPrMerge
	case event.TypeWorkflowFailure:
		return (cfg == nil || cfg.NotifyWorkflowFailure) && u.NotifyWorkflowFailure
	}
	return false
}

// shouldNotify is the pure gating predicate: the global admin switch, the
// per-type admin default, the user's per-type preference, and the user's
// master switch must all be on.
func shouldNotify(cfg *ent.AdminConfig, u *ent.User, t event.Type) bool {
	if u == nil || !u.NotificationsEnabled {
		return false
	}
	if cfg != nil && !cfg.NotificationsEnabled {
		return false
	}
	return eventTypeEnabled(cfg, u, t)
}

// notificationEvent is the JSON payload sent to browser clients.
type notificationEvent struct {
	ID        int    `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Repo      string `json:"repo"`
	Timestamp string `json:"timestamp"`
}

type notificationMessage struct {
	Type  string            `json:"type"`
	Event notificationEvent `json:"event"`
}

// NotifyEvent delivers one event to one user over every enabled channel.
func (n *Notifier) NotifyEvent(ctx context.Context, u *ent.User, repoFull string, ev *ent.Event) {
	if n == nil || n.client == nil || u == nil || ev == nil {
		return
	}

	cfg, err := n.client.AdminConfig.Get(ctx, 1)
	if err != nil {
		if !ent.IsNotFound(err) {
			log.Printf("notify: error loading admin config: %v", err)
			return
		}
		cfg = nil // missing row: defaults (all enabled)
	}

	if !shouldNotify(cfg, u, ev.Type) {
		return
	}

	if u.NotifyWeb {
		n.notifyWeb(ctx, u, repoFull, ev)
	}
	if u.NotifyAndroid && cfg != nil {
		n.notifyAndroid(ctx, u, repoFull, ev, cfg)
	}
}

// notifyWeb pushes a JSON message to the user's WebSocket connections.
func (n *Notifier) notifyWeb(_ context.Context, u *ent.User, repoFull string, ev *ent.Event) {
	if n.hub == nil {
		return
	}
	msg, err := json.Marshal(notificationMessage{
		Type: "notification",
		Event: notificationEvent{
			ID:        ev.ID,
			Type:      string(ev.Type),
			Title:     ev.Title,
			URL:       ev.URL,
			Repo:      repoFull,
			Timestamp: ev.Timestamp.Format(time.RFC3339),
		},
	})
	if err != nil {
		log.Printf("notify: error encoding web notification: %v", err)
		return
	}
	n.hub.BroadcastToUser(int64(u.ID), msg)
}

// fcmClient returns a cached FCM client for the given configuration, creating
// one on first use. The cache key includes the service-account JSON so a
// credential change automatically produces a new client.
func (n *Notifier) fcmClient(projectID, serviceAccountJSON string) *fcm.Client {
	key := projectID + "\x00" + serviceAccountJSON
	n.mu.Lock()
	defer n.mu.Unlock()
	if c, ok := n.clients[key]; ok {
		return c
	}
	c := fcm.New(projectID, serviceAccountJSON)
	n.clients[key] = c
	return c
}

// notifyAndroid sends the event to every registered Android device token.
func (n *Notifier) notifyAndroid(ctx context.Context, u *ent.User, repoFull string, ev *ent.Event, cfg *ent.AdminConfig) {
	if cfg.FcmProjectID == "" || cfg.FcmServiceAccountJSON == "" {
		return
	}
	serviceAccount, err := secrets.Decrypt(cfg.FcmServiceAccountJSON)
	if err != nil {
		log.Printf("notify: FCM service account could not be decrypted")
		return
	}
	client := n.fcmClient(cfg.FcmProjectID, serviceAccount)
	if client == nil {
		return
	}

	tokens, err := n.client.DeviceToken.Query().
		Where(
			devicetoken.HasUserWith(user.ID(u.ID)),
			devicetoken.PlatformEQ(devicetoken.PlatformAndroid),
		).
		All(ctx)
	if err != nil {
		log.Printf("notify: error loading device tokens for user %d: %v", u.ID, err)
		return
	}

	body := fmt.Sprintf("%s · %s", ev.Type, repoFull)
	data := map[string]string{
		"event_id": fmt.Sprintf("%d", ev.ID),
		"type":     string(ev.Type),
		"url":      ev.URL,
		"repo":     repoFull,
	}
	for _, dt := range tokens {
		if err := client.Send(ctx, dt.Token, ev.Title, body, data); err != nil {
			if errors.Is(err, fcm.ErrInvalidToken) {
				if delErr := n.client.DeviceToken.DeleteOneID(dt.ID).Exec(ctx); delErr != nil {
					log.Printf("notify: error deleting stale device token for user %d: %v", u.ID, delErr)
				}
				continue
			}
			// Never log the token itself.
			log.Printf("notify: FCM send failed for user %d (platform=android)", u.ID)
		}
	}
}
