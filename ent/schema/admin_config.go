package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AdminConfig holds application-wide admin settings (singleton row).
type AdminConfig struct {
	ent.Schema
}

func (AdminConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Default(1), // singleton: always ID 1
		field.String("otel_endpoint").Optional(),
		field.Bool("traces_enabled").Default(false),
		field.Bool("metrics_enabled").Default(false),
		field.Bool("logs_enabled").Default(false),
		field.String("log_severity").Default("warning"),
		// Gotify push notification settings for the deploy subsystem.
		// Managed from the admin panel; env vars (GOTIFY_URL/GOTIFY_TOKEN)
		// remain the fallback when unset.
		field.String("gotify_url").Optional(),
		field.String("gotify_token").Optional(),
		// Telegram push notification settings (Bot API). Managed from the
		// admin panel; no env fallback.
		field.String("telegram_bot_token").Optional(),
		field.String("telegram_chat_id").Optional(),
		// Activity-feed notification settings. The global switch and the
		// per-type defaults gate delivery; FCM credentials enable Android
		// push. The service-account JSON is stored encrypted.
		field.Bool("notifications_enabled").Default(true),
		field.Bool("notify_release").Default(true),
		field.Bool("notify_pr_merge").Default(true),
		field.Bool("notify_workflow_failure").Default(true),
		field.String("fcm_project_id").Optional(),
		field.String("fcm_service_account_json").Optional(),
		field.String("fcm_api_key").Optional(),
		field.String("fcm_app_id").Optional(),
		field.String("fcm_sender_id").Optional(),
		field.String("fcm_storage_bucket").Optional(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
