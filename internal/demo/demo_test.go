package demo_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"gitlens/ent"
	"gitlens/ent/apitoken"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/config"
	"gitlens/internal/demo"
	"gitlens/internal/middleware"

	_ "github.com/mattn/go-sqlite3"
)

func newDemoTestClient(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/demo.db?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func enabledConfig() config.Demo {
	return config.Demo{Enabled: true, Login: "demo", Token: "glt_demo_public_preview_2026"}
}

func TestEnsure_IdempotentAndSeedsOnce(t *testing.T) {
	client := newDemoTestClient(t)
	ctx := context.Background()
	cfg := enabledConfig()

	u, err := demo.Ensure(ctx, client, cfg)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if u == nil {
		t.Fatal("first Ensure returned nil user")
	}
	if u.IsAdmin {
		t.Fatal("demo user must not be admin")
	}

	repos, err := client.Repository.Query().Where(repository.HasUserWith(user.ID(u.ID))).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 3 {
		t.Fatalf("want 3 repositories, got %d", len(repos))
	}

	for _, r := range repos {
		if r.PullRequests == "" {
			t.Fatalf("repo %s has no pull request JSON", r.FullName)
		}
		var prs []struct {
			N  int    `json:"n"`
			T  string `json:"t"`
			A  string `json:"a"`
			C  string `json:"c"`
			H  string `json:"h"`
			HR string `json:"hr"`
			BR string `json:"br"`
			MS string `json:"ms"`
			BS string `json:"bs"`
		}
		if err := json.Unmarshal([]byte(r.PullRequests), &prs); err != nil {
			t.Fatalf("repo %s pull request JSON invalid: %v", r.FullName, err)
		}
		if len(prs) == 0 {
			t.Fatalf("repo %s pull request JSON empty", r.FullName)
		}
	}

	counts := func() (int, int, int) {
		commits, err := client.CommitActivity.Query().Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		metrics, err := client.MetricSnapshot.Query().Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		events, err := client.Event.Query().Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return commits, metrics, events
	}
	c1, m1, e1 := counts()
	if c1 == 0 || m1 == 0 || e1 == 0 {
		t.Fatalf("expected seeded activity/metrics/events, got %d/%d/%d", c1, m1, e1)
	}

	u2, err := demo.Ensure(ctx, client, cfg)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if u2.ID != u.ID {
		t.Fatalf("second Ensure created a different user: %d vs %d", u2.ID, u.ID)
	}
	if u2.IsAdmin {
		t.Fatal("demo user must still not be admin after second Ensure")
	}

	demoUsers, err := client.User.Query().Where(user.GithubID(demo.GithubID)).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if demoUsers != 1 {
		t.Fatalf("want exactly 1 demo user, got %d", demoUsers)
	}

	repos2, err := client.Repository.Query().Where(repository.HasUserWith(user.ID(u.ID))).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repos2 != 3 {
		t.Fatalf("want 3 repositories after second Ensure, got %d", repos2)
	}

	c2, m2, e2 := counts()
	if c1 != c2 || m1 != m2 || e1 != e2 {
		t.Fatalf("seed rows duplicated: commits %d->%d metrics %d->%d events %d->%d", c1, c2, m1, m2, e1, e2)
	}

	tokens, err := client.ApiToken.Query().Where(apitoken.UserID(u.ID)).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 1 {
		t.Fatalf("want exactly 1 demo api token, got %d", tokens)
	}

	tokenID, userID, err := middleware.VerifyToken(ctx, client, cfg.Token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if tokenID == 0 || userID != int64(u.ID) {
		t.Fatalf("token resolved to user %d (token %d), want %d", userID, tokenID, u.ID)
	}

	// Repairs an accidental admin promotion.
	if _, err := client.User.UpdateOneID(u.ID).SetIsAdmin(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := demo.Ensure(ctx, client, cfg); err != nil {
		t.Fatal(err)
	}
	repaired, err := client.User.Get(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.IsAdmin {
		t.Fatal("Ensure did not demote the demo user")
	}
}

func TestEnsure_DisabledCreatesNothing(t *testing.T) {
	client := newDemoTestClient(t)
	ctx := context.Background()

	u, err := demo.Ensure(ctx, client, config.Demo{Enabled: false, Login: "demo", Token: "glt_demo_public_preview_2026"})
	if err != nil {
		t.Fatalf("Ensure disabled: %v", err)
	}
	if u != nil {
		t.Fatal("disabled demo returned a user")
	}

	users, err := client.User.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repos, err := client.Repository.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := client.ApiToken.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := client.CommitActivity.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := client.MetricSnapshot.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Event.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for name, count := range map[string]int{
		"users":   users,
		"repos":   repos,
		"tokens":  tokens,
		"commits": commits,
		"metrics": metrics,
		"events":  events,
	} {
		if count != 0 {
			t.Fatalf("disabled demo created %d %s", count, name)
		}
	}
}
