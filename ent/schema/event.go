package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Event struct {
	ent.Schema
}

func (Event) Fields() []ent.Field {
	return []ent.Field{
		field.Int("repo_id"),
		field.Enum("type").Values("release", "workflow_failure", "pr_merge"),
		field.String("title"),
		field.String("url"),
		field.String("metadata").Optional(),
		field.Time("timestamp"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Event) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("timestamp"),
		index.Fields("repo_id", "timestamp"),
		index.Fields("type", "timestamp"),
	}
}
