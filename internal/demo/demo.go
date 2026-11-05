// Package demo owns the shared public demo account used by app-store reviewers.
// The account is deliberately non-admin and holds only synthetic sample data;
// no real GitHub credentials are ever stored for it.
package demo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"gitlens/ent"
	"gitlens/ent/apitoken"
	"gitlens/ent/commitactivity"
	"gitlens/ent/event"
	"gitlens/ent/metricsnapshot"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/config"
)

// GithubID is the negative marker for the demo user. OIDC users get high
// positive IDs and real GitHub IDs are positive, so -1 cannot collide.
const GithubID int64 = -1

const demoOwner = "demo-lens"

// demoPR mirrors the JSON shape the dashboard expects for repository.pull_requests.
type demoPR struct {
	Number         int    `json:"n"`
	Title          string `json:"t"`
	Author         string `json:"a"`
	CreatedAt      string `json:"c"`
	HTMLURL        string `json:"h"`
	HeadRef        string `json:"hr"`
	BaseRef        string `json:"br"`
	MergeableState string `json:"ms"`
	BuildStatus    string `json:"bs"`
}

type repoSeed struct {
	githubID       int64
	name           string
	description    string
	language       string
	commitMessage  string
	releaseTag     string
	releaseName    string
	workflowStatus string
	commits        int
	feat           int
	fix            int
	docs           int
	chore          int
	other          int
	releases       int
	workflowOK     int
	workflowFail   int
	openPRs        int
	leadHours      float64
	prs            []demoPR
}

func repoSeeds() []repoSeed {
	now := time.Now()
	ago := func(hours int) string {
		return now.Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	}
	return []repoSeed{
		{
			githubID:       -2,
			name:           "pulse-api",
			description:    "High-throughput Go API that ingests and aggregates device telemetry.",
			language:       "Go",
			commitMessage:  "feat: add per-tenant rate limiting",
			releaseTag:     "v1.8.0",
			releaseName:    "Pulse API 1.8.0",
			workflowStatus: "success",
			commits:        1842,
			feat:           46,
			fix:            21,
			docs:           9,
			chore:          14,
			other:          7,
			releases:       12,
			workflowOK:     148,
			workflowFail:   6,
			openPRs:        4,
			leadHours:      18.4,
			prs: []demoPR{
				{Number: 212, Title: "feat: add per-tenant rate limiting", Author: "demo-lens", CreatedAt: ago(9), HTMLURL: "https://github.com/demo-lens/pulse-api/pull/212", HeadRef: "feat/rate-limit", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 208, Title: "fix: flush buffered spans on shutdown", Author: "demo-lens", CreatedAt: ago(31), HTMLURL: "https://github.com/demo-lens/pulse-api/pull/208", HeadRef: "fix/flush-spans", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 205, Title: "docs: document aggregation window", Author: "demo-lens", CreatedAt: ago(52), HTMLURL: "https://github.com/demo-lens/pulse-api/pull/205", HeadRef: "docs/window", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 201, Title: "chore: bump golangci-lint", Author: "demo-lens", CreatedAt: ago(73), HTMLURL: "https://github.com/demo-lens/pulse-api/pull/201", HeadRef: "chore/lint", BaseRef: "main", MergeableState: "unknown", BuildStatus: "failure"},
			},
		},
		{
			githubID:       -3,
			name:           "orbit-web",
			description:    "TypeScript dashboard for exploring satellite pass predictions.",
			language:       "TypeScript",
			commitMessage:  "feat(ui): add pass timeline scrubber",
			releaseTag:     "v0.9.2",
			releaseName:    "Orbit Web 0.9.2",
			workflowStatus: "failure",
			commits:        927,
			feat:           38,
			fix:            19,
			docs:           6,
			chore:          11,
			other:          5,
			releases:       7,
			workflowOK:     96,
			workflowFail:   12,
			openPRs:        6,
			leadHours:      26.7,
			prs: []demoPR{
				{Number: 144, Title: "feat(ui): add pass timeline scrubber", Author: "demo-lens", CreatedAt: ago(5), HTMLURL: "https://github.com/demo-lens/orbit-web/pull/144", HeadRef: "feat/scrubber", BaseRef: "main", MergeableState: "clean", BuildStatus: "failure"},
				{Number: 141, Title: "fix: keep tooltip inside viewport", Author: "demo-lens", CreatedAt: ago(20), HTMLURL: "https://github.com/demo-lens/orbit-web/pull/141", HeadRef: "fix/tooltip", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 138, Title: "docs: refresh local setup guide", Author: "demo-lens", CreatedAt: ago(44), HTMLURL: "https://github.com/demo-lens/orbit-web/pull/138", HeadRef: "docs/setup", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 136, Title: "chore: migrate to pnpm workspaces", Author: "demo-lens", CreatedAt: ago(66), HTMLURL: "https://github.com/demo-lens/orbit-web/pull/136", HeadRef: "chore/pnpm", BaseRef: "main", MergeableState: "dirty", BuildStatus: "failure"},
			},
		},
		{
			githubID:       -4,
			name:           "signal-cli",
			description:    "Python CLI for decoding and archiving radio signal captures.",
			language:       "Python",
			commitMessage:  "fix: handle truncated IQ frames",
			releaseTag:     "v2.3.1",
			releaseName:    "Signal CLI 2.3.1",
			workflowStatus: "success",
			commits:        613,
			feat:           24,
			fix:            17,
			docs:           8,
			chore:          9,
			other:          4,
			releases:       9,
			workflowOK:     71,
			workflowFail:   4,
			openPRs:        2,
			leadHours:      12.9,
			prs: []demoPR{
				{Number: 88, Title: "fix: handle truncated IQ frames", Author: "demo-lens", CreatedAt: ago(14), HTMLURL: "https://github.com/demo-lens/signal-cli/pull/88", HeadRef: "fix/iq-frames", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 84, Title: "feat: add WAV export", Author: "demo-lens", CreatedAt: ago(40), HTMLURL: "https://github.com/demo-lens/signal-cli/pull/84", HeadRef: "feat/wav-export", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 80, Title: "docs: explain sample rates", Author: "demo-lens", CreatedAt: ago(61), HTMLURL: "https://github.com/demo-lens/signal-cli/pull/80", HeadRef: "docs/sample-rates", BaseRef: "main", MergeableState: "clean", BuildStatus: "success"},
				{Number: 79, Title: "chore: pin ruff version", Author: "demo-lens", CreatedAt: ago(78), HTMLURL: "https://github.com/demo-lens/signal-cli/pull/79", HeadRef: "chore/ruff", BaseRef: "main", MergeableState: "unknown", BuildStatus: "success"},
			},
		},
	}
}

// Ensure creates or refreshes the demo account and its sample data. It is
// idempotent: calling it repeatedly repairs the non-admin flag and token but
// never duplicates repositories, activity, metrics or events.
func Ensure(ctx context.Context, client *ent.Client, cfg config.Demo) (*ent.User, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if client == nil {
		return nil, errors.New("demo: nil ent client")
	}

	u, err := client.User.Query().Where(user.GithubID(GithubID)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		u, err = client.User.Create().
			SetGithubID(GithubID).
			SetLogin(cfg.Login).
			SetName("Demo Lens").
			SetAccessToken("demo").
			SetIsAdmin(false).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("demo: create user: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("demo: query user: %w", err)
	default:
		u, err = client.User.UpdateOneID(u.ID).
			SetLogin(cfg.Login).
			SetName("Demo Lens").
			SetAccessToken("demo").
			SetIsAdmin(false).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("demo: update user: %w", err)
		}
	}

	if err := ensureToken(ctx, client, u, cfg.Token); err != nil {
		return nil, err
	}

	repoCount, err := client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("demo: count repositories: %w", err)
	}
	if repoCount == 0 {
		if err := seedRepositories(ctx, client, u); err != nil {
			return nil, err
		}
	}

	repos, err := client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Asc(repository.FieldGithubID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("demo: load repositories: %w", err)
	}

	for _, r := range repos {
		if err := seedCommitActivity(ctx, client, r); err != nil {
			return nil, err
		}
		if err := seedMetricSnapshots(ctx, client, r); err != nil {
			return nil, err
		}
	}
	if err := seedEvents(ctx, client, repos); err != nil {
		return nil, err
	}

	log.Printf("Demo account enabled (login %q)", cfg.Login)
	return u, nil
}

func ensureToken(ctx context.Context, client *ent.Client, u *ent.User, token string) error {
	hash := tokenHash(token)
	existing, err := client.ApiToken.Query().Where(apitoken.TokenHash(hash)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		_, err = client.ApiToken.Create().
			SetUserID(u.ID).
			SetName("Public demo").
			SetTokenHash(hash).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("demo: create api token: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("demo: query api token: %w", err)
	case existing.UserID != u.ID:
		return errors.New("demo: configured token hash belongs to another user")
	default:
		return nil
	}
}

func seedRepositories(ctx context.Context, client *ent.Client, u *ent.User) error {
	now := time.Now()
	for i, seed := range repoSeeds() {
		prJSON, err := json.Marshal(seed.prs)
		if err != nil {
			return fmt.Errorf("demo: encode pull requests: %w", err)
		}
		commitDate := now.Add(-time.Duration(i+2) * time.Hour)
		releaseDate := now.Add(-time.Duration((i+1)*24) * time.Hour)
		_, err = client.Repository.Create().
			SetGithubID(seed.githubID).
			SetOwner(demoOwner).
			SetName(seed.name).
			SetFullName(demoOwner + "/" + seed.name).
			SetDescription(seed.description).
			SetHTMLURL("https://github.com/" + demoOwner + "/" + seed.name).
			SetLanguage(seed.language).
			SetDefaultBranch("main").
			SetLatestCommitSha(demoSHA(seed.githubID)).
			SetLatestCommitMessage(seed.commitMessage).
			SetLatestCommitDate(commitDate).
			SetLatestReleaseTag(seed.releaseTag).
			SetLatestReleaseName(seed.releaseName).
			SetLatestReleaseDate(releaseDate).
			SetLatestReleaseConclusion(releaseConclusion(seed.workflowStatus)).
			SetWorkflowStatus(seed.workflowStatus).
			SetTotalCommitsFetched(seed.commits).
			SetFeatCount(seed.feat).
			SetFixCount(seed.fix).
			SetDocsCount(seed.docs).
			SetChoreCount(seed.chore).
			SetOtherCommitCount(seed.other).
			SetReleaseCount(seed.releases).
			SetAvgLeadTimeHours(seed.leadHours).
			SetWorkflowSuccessCount(seed.workflowOK).
			SetWorkflowFailureCount(seed.workflowFail).
			SetOpenPrCount(seed.openPRs).
			SetPullRequests(string(prJSON)).
			SetBackfillStatus("done").
			SetSyncedAt(now).
			SetUserID(u.ID).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("demo: create repository %s: %w", seed.name, err)
		}
	}
	return nil
}

func releaseConclusion(status string) string {
	if status == "failure" {
		return "failure"
	}
	return "success"
}

func demoSHA(seed int64) string {
	base := "3f9c2a7e5b1d4068"
	if seed == -2 {
		return base + "a1c9d2e7"
	}
	if seed == -3 {
		return base + "b7f4e6a1"
	}
	return base + "c2d8f3b6"
}

func seedCommitActivity(ctx context.Context, client *ent.Client, r *ent.Repository) error {
	n, err := client.CommitActivity.Query().Where(commitactivity.RepoID(r.ID)).Count(ctx)
	if err != nil {
		return fmt.Errorf("demo: count commit activity: %w", err)
	}
	if n > 0 {
		return nil
	}

	now := time.Now().UTC()
	offset := int(r.GithubID*-1) + 3
	for i := 89; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		date := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		count := (i*7 + offset*3) % 10
		// Weekends are quieter but not empty.
		if wd := date.Weekday(); wd == time.Saturday || wd == time.Sunday {
			count = count / 2
		}
		_, err := client.CommitActivity.Create().
			SetRepoID(r.ID).
			SetDate(date).
			SetCommitCount(count).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("demo: create commit activity: %w", err)
		}
	}
	return nil
}

func seedMetricSnapshots(ctx context.Context, client *ent.Client, r *ent.Repository) error {
	n, err := client.MetricSnapshot.Query().Where(metricsnapshot.RepoID(r.ID)).Count(ctx)
	if err != nil {
		return fmt.Errorf("demo: count metric snapshots: %w", err)
	}
	if n > 0 {
		return nil
	}

	now := time.Now().UTC()
	offset := int(r.GithubID * -1)
	for i := 29; i >= 0; i-- {
		ts := now.AddDate(0, 0, -i)
		scale := 30 - i
		_, err := client.MetricSnapshot.Create().
			SetRepoID(r.ID).
			SetTimestamp(ts).
			SetFeatCount(maxInt(0, r.FeatCount-scale/3)).
			SetFixCount(maxInt(0, r.FixCount-scale/4)).
			SetDocsCount(maxInt(0, r.DocsCount-scale/6)).
			SetChoreCount(maxInt(0, r.ChoreCount-scale/6)).
			SetOtherCommitCount(maxInt(0, r.OtherCommitCount-scale/8)).
			SetTotalCommitsFetched(maxInt(0, r.TotalCommitsFetched-scale*4)).
			SetReleaseCount(maxInt(0, r.ReleaseCount-scale/5)).
			SetAvgLeadTimeHours(float64(offset) + r.AvgLeadTimeHours - float64(scale)/10).
			SetWorkflowSuccessCount(maxInt(0, r.WorkflowSuccessCount-scale/2)).
			SetWorkflowFailureCount(maxInt(0, r.WorkflowFailureCount-scale/10)).
			SetWorkflowStatus(metricStatus(r.WorkflowStatus, i)).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("demo: create metric snapshot: %w", err)
		}
	}
	return nil
}

func metricStatus(current string, daysAgo int) string {
	if daysAgo%11 == 0 && current == "success" {
		return "failure"
	}
	if daysAgo%9 == 0 && current == "failure" {
		return "success"
	}
	return current
}

func seedEvents(ctx context.Context, client *ent.Client, repos []*ent.Repository) error {
	if len(repos) == 0 {
		return nil
	}
	ids := make([]int, 0, len(repos))
	for _, r := range repos {
		ids = append(ids, r.ID)
	}
	n, err := client.Event.Query().Where(event.RepoIDIn(ids...)).Count(ctx)
	if err != nil {
		return fmt.Errorf("demo: count events: %w", err)
	}
	if n > 0 {
		return nil
	}

	now := time.Now()
	specs := []struct {
		repo   int
		kind   event.Type
		title  string
		url    string
		meta   string
		hoursA int
	}{
		{0, event.TypeRelease, "Released Pulse API 1.8.0", "https://github.com/demo-lens/pulse-api/releases/tag/v1.8.0", `{"tag":"v1.8.0"}`, 3},
		{1, event.TypeWorkflowFailure, "CI failed on orbit-web: typecheck", "https://github.com/demo-lens/orbit-web/actions", `{"branch":"feat/scrubber"}`, 6},
		{2, event.TypePrMerge, "Merged #88: handle truncated IQ frames", "https://github.com/demo-lens/signal-cli/pull/88", `{"number":88}`, 14},
		{0, event.TypePrMerge, "Merged #212: per-tenant rate limiting", "https://github.com/demo-lens/pulse-api/pull/212", `{"number":212}`, 27},
		{1, event.TypeRelease, "Released Orbit Web 0.9.2", "https://github.com/demo-lens/orbit-web/releases/tag/v0.9.2", `{"tag":"v0.9.2"}`, 40},
		{2, event.TypeWorkflowFailure, "CI failed on signal-cli: lint", "https://github.com/demo-lens/signal-cli/actions", `{"branch":"fix/iq-frames"}`, 55},
		{0, event.TypeWorkflowFailure, "CI failed on pulse-api: integration", "https://github.com/demo-lens/pulse-api/actions", `{"branch":"main"}`, 74},
		{1, event.TypePrMerge, "Merged #141: keep tooltip inside viewport", "https://github.com/demo-lens/orbit-web/pull/141", `{"number":141}`, 96},
	}
	for i, s := range specs {
		r := repos[s.repo%len(repos)]
		_, err := client.Event.Create().
			SetRepoID(r.ID).
			SetType(s.kind).
			SetTitle(s.title).
			SetURL(s.url).
			SetMetadata(s.meta).
			SetTimestamp(now.Add(-time.Duration(s.hoursA+i) * time.Hour)).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("demo: create event: %w", err)
		}
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// tokenHash mirrors middleware.HashToken. It lives here to keep the demo
// package independent of the middleware/auth import chain, which would
// otherwise create a cycle (middleware → auth → demo). The algorithm is the
// same SHA-256 hex digest, so middleware.VerifyToken resolves these tokens.
func tokenHash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
