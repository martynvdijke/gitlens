package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type CommitActivity struct {
	ent.Schema
}

func (CommitActivity) Fields() []ent.Field {
	return []ent.Field{
		field.Int("repo_id"),
		field.Time("date"), // date-only semantics (YYYY-MM-DD UTC)
		field.Int("commit_count"),
	}
}

func (CommitActivity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("repo_id", "date").Unique(),
		index.Fields("date"),
	}
}
