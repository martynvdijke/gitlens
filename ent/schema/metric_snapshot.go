package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type MetricSnapshot struct {
	ent.Schema
}

func (MetricSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int("repo_id"),
		field.Time("timestamp"),
		field.Int("feat_count").Optional(),
		field.Int("fix_count").Optional(),
		field.Int("docs_count").Optional(),
		field.Int("chore_count").Optional(),
		field.Int("other_commit_count").Optional(),
		field.Int("total_commits_fetched").Optional(),
		field.Int("release_count").Optional(),
		field.Float("avg_lead_time_hours").Optional(),
		field.Int("workflow_success_count").Optional(),
		field.Int("workflow_failure_count").Optional(),
		field.String("workflow_status").Optional(),
	}
}

func (MetricSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("repo_id", "timestamp"),
	}
}
