package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Repository struct {
	ent.Schema
}

func (Repository) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("github_id").Unique(),
		field.String("owner"),
		field.String("name"),
		field.String("full_name"),
		field.String("description").Optional(),
		field.String("html_url"),
		field.String("language").Optional(),
		field.String("default_branch"),
		field.String("latest_commit_sha").Optional(),
		field.String("latest_commit_message").Optional(),
		field.Time("latest_commit_date").Optional(),
		field.String("latest_release_tag").Optional(),
		field.String("latest_release_name").Optional(),
		field.Time("latest_release_date").Optional(),
		field.String("workflow_status").Optional().Default("unknown"),
		field.Int64("workflow_run_id").Optional(),
		field.Time("synced_at").Optional(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Repository) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("repositories").Unique().Required(),
	}
}
