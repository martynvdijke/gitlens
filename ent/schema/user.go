package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("github_id").Unique(),
		field.String("login"),
		field.String("avatar_url").Optional(),
		field.String("name").Optional(),
		field.String("access_token"),
		field.Int("sync_interval_minutes").Default(15),
		field.Time("synced_at").Optional(),
		field.Time("created_at").Default(time.Now),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("repositories", Repository.Type),
	}
}
