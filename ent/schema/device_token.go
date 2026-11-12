package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DeviceToken stores an FCM registration token for a user's Android or web
// client. A single token (the FCM registration token) is the uniqueness key:
// when a device signs in as a different user the same row is reassigned.
type DeviceToken struct {
	ent.Schema
}

func (DeviceToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("token").Unique(),
		field.Enum("platform").Values("android", "web"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("last_seen_at").Default(time.Now),
	}
}

func (DeviceToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("device_tokens").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (DeviceToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
