package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
		field.String("umami_url").Optional(),
		field.String("umami_site_id").Optional(),
		field.Bool("is_admin").Default(false),
		field.Time("synced_at").Optional(),
		field.Time("created_at").Default(time.Now),

		// Forgejo connection — all nullable. A user can connect
		// both providers or only one.
		field.Int64("forgejo_id").Optional(),
		field.String("forgejo_login").Optional(),
		field.String("forgejo_avatar_url").Optional(),
		field.String("forgejo_name").Optional(),
		field.String("forgejo_access_token").Optional(),
		field.String("forgejo_url").Optional(),

		// JSON array of github repo full_names the user has
		// acknowledged in the cross-provider warning. Stored as a
		// plain string (SQLite has no native JSON type);
		// serialization is handled by the handler.
		field.String("dismissed_forgejo_warning_for").Optional(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("repositories", Repository.Type),
	}
}

// Indexes for the User schema. forgejo_id alone is not unique (a user
// could have the same numeric ID on two different Forgejo instances),
// so the composite (forgejo_id, forgejo_url) is the uniqueness key.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("forgejo_id", "forgejo_url").Unique(),
	}
}

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema

