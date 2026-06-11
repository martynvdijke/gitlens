package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Repository struct {
	ent.Schema
}

func (Repository) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("github_id"),
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
		field.Int("total_commits_fetched").Optional(),
		field.Int("feat_count").Optional(),
		field.Int("fix_count").Optional(),
		field.Int("docs_count").Optional(),
		field.Int("chore_count").Optional(),
		field.Int("other_commit_count").Optional(),
		field.Int("release_count").Optional(),
		field.Float("avg_lead_time_hours").Optional(),
		field.Int("workflow_success_count").Optional(),
		field.Int("workflow_failure_count").Optional(),
		field.Int("open_pr_count").Optional(),
		field.String("pull_requests").Optional(),
		field.String("latest_release_conclusion").Optional().Default("unknown"),
		field.Time("synced_at").Optional(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),

		// Provider discriminator: "github" (default) or "forgejo".
		field.String("provider").Default("github"),

		// Forgejo-specific fields. Populated when provider=="forgejo".
		field.Int64("forgejo_id").Optional(),
		field.String("forgejo_owner").Optional(),
		field.String("forgejo_name").Optional(),
		field.String("forgejo_full_name").Optional(),
		field.String("forgejo_html_url").Optional(),
		field.String("forgejo_url").Optional(),
	}
}

func (Repository) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("repositories").Unique().Required(),
	}
}

// Composite unique indexes so the same github_id / forgejo_id can be
// tracked by multiple users but not duplicated for the same user. The
// edge FK is referenced via .Edges("user") so the underlying column
// doesn't need to be re-declared as a field.
func (Repository) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("github_id").Edges("user").Unique(),
		index.Fields("forgejo_id").Edges("user").Unique(),
		// forgejo_full_name is queried when computing the
		// cross-provider warning; an index keeps the lookup fast.
		index.Fields("forgejo_full_name"),
	}
}

