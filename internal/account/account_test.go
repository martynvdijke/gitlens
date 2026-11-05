package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"gitlens/ent"
	"gitlens/ent/apitoken"
	"gitlens/ent/commitactivity"
	"gitlens/ent/event"
	"gitlens/ent/metricsnapshot"
	"gitlens/ent/repository"
	"gitlens/ent/user"

	_ "github.com/mattn/go-sqlite3"
)

type fakeGitHubRevoker struct {
	tokens []string
	err    error
}

func (f *fakeGitHubRevoker) RevokeToken(_ context.Context, token string) error {
	f.tokens = append(f.tokens, token)
	return f.err
}

type fakeForgejoRevoker struct {
	bases  []string
	tokens []string
	err    error
}

func (f *fakeForgejoRevoker) RevokeToken(_ context.Context, base, token string) error {
	f.bases = append(f.bases, base)
	f.tokens = append(f.tokens, token)
	return f.err
}

func newAccountTestService(t *testing.T) (*Service, *ent.Client, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/account.db?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, expires_at DATETIME NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return New(client, db), client, db
}

func nowDate() time.Time { return time.Now().UTC() }

func TestDelete_RemovesAccountAndCredentials(t *testing.T) {
	svc, client, db := newAccountTestService(t)
	ctx := context.Background()

	keep, err := client.User.Create().SetGithubID(1).SetLogin("admin").SetAccessToken("gh-admin").SetIsAdmin(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := client.User.Create().
		SetGithubID(2).
		SetLogin("target").
		SetAccessToken("gh-target-token").
		SetForgejoID(9).
		SetForgejoLogin("target-fj").
		SetForgejoAccessToken("fj-target-token").
		SetForgejoURL("https://code.example.org").
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('s1', ?, '2099-01-01 00:00:00')`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApiToken.Create().SetUserID(u.ID).SetName("ci").SetTokenHash("hash-target").Save(ctx); err != nil {
		t.Fatal(err)
	}
	repo, err := client.Repository.Create().
		SetGithubID(100).SetOwner("target").SetName("app").SetFullName("target/app").
		SetHTMLURL("https://github.com/target/app").SetDefaultBranch("main").SetUserID(u.ID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CommitActivity.Create().SetRepoID(repo.ID).SetDate(nowDate()).SetCommitCount(3).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Event.Create().SetRepoID(repo.ID).SetType("release").SetTitle("v1").SetURL("https://example.org").SetTimestamp(nowDate()).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MetricSnapshot.Create().SetRepoID(repo.ID).SetTimestamp(nowDate()).SetFeatCount(2).Save(ctx); err != nil {
		t.Fatal(err)
	}
	// Another user's copy of the same repository, and a repository that does not
	// belong to the target, must survive.
	otherRepo, err := client.Repository.Create().
		SetGithubID(100).SetOwner("target").SetName("app").SetFullName("target/app").
		SetHTMLURL("https://github.com/target/app").SetDefaultBranch("main").SetUserID(keep.ID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.MetricSnapshot.Create().SetRepoID(otherRepo.ID).SetTimestamp(nowDate()).SetFeatCount(9).Save(ctx); err != nil {
		t.Fatal(err)
	}

	gh := &fakeGitHubRevoker{}
	fj := &fakeForgejoRevoker{}
	svc.GitHubRevoker = gh
	svc.ForgejoRevoker = fj

	if err := svc.Delete(ctx, int64(u.ID)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(gh.tokens) != 1 || gh.tokens[0] != "gh-target-token" {
		t.Fatalf("expected GitHub token revocation, got %v", gh.tokens)
	}
	if len(fj.tokens) != 1 || fj.tokens[0] != "fj-target-token" || fj.bases[0] != "https://code.example.org" {
		t.Fatalf("expected Forgejo token revocation, got %v %v", fj.bases, fj.tokens)
	}
	if _, err := client.User.Get(ctx, u.ID); !ent.IsNotFound(err) {
		t.Fatalf("expected user row deleted, got err=%v", err)
	}
	if n, _ := client.ApiToken.Query().Where(apitoken.UserID(u.ID)).Count(ctx); n != 0 {
		t.Fatalf("expected tokens deleted, got %d", n)
	}
	if n, _ := client.Repository.Query().Where(repository.HasUserWith(user.IDEQ(u.ID))).Count(ctx); n != 0 {
		t.Fatalf("expected tracked repos deleted, got %d", n)
	}
	if n, _ := client.CommitActivity.Query().Where(commitactivity.RepoID(repo.ID)).Count(ctx); n != 0 {
		t.Fatalf("expected commit activity deleted, got %d", n)
	}
	if n, _ := client.Event.Query().Where(event.RepoID(repo.ID)).Count(ctx); n != 0 {
		t.Fatalf("expected events deleted, got %d", n)
	}
	if n, _ := client.MetricSnapshot.Query().Where(metricsnapshot.RepoID(repo.ID)).Count(ctx); n != 0 {
		t.Fatalf("expected metric snapshots deleted, got %d", n)
	}
	if n, _ := client.MetricSnapshot.Query().Where(metricsnapshot.RepoID(otherRepo.ID)).Count(ctx); n != 1 {
		t.Fatalf("expected other user's metrics preserved, got %d", n)
	}
	var sessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, u.ID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("expected sessions deleted, got %d", sessions)
	}
}

func TestDelete_RefusesLastAdmin(t *testing.T) {
	svc, client, _ := newAccountTestService(t)
	ctx := context.Background()
	u, err := client.User.Create().SetGithubID(1).SetLogin("only-admin").SetAccessToken("token").SetIsAdmin(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, int64(u.ID)); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin, got %v", err)
	}
	if _, err := client.User.Get(ctx, u.ID); err != nil {
		t.Fatalf("last admin must survive: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc, _, _ := newAccountTestService(t)
	if err := svc.Delete(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestExport_OmitsSecrets(t *testing.T) {
	svc, client, _ := newAccountTestService(t)
	ctx := context.Background()
	u, err := client.User.Create().
		SetGithubID(7).SetLogin("exporter").SetName("Exporter").
		SetAccessToken("super-secret-github-token").
		SetOidcSub("issuer|sub").SetOidcEmail("person@example.com").
		SetUmamiURL("https://umami.example.org").SetUmamiSiteID("site-1").
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApiToken.Create().SetUserID(u.ID).SetName("ci").SetTokenHash("secret-hash").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Repository.Create().
		SetGithubID(100).SetOwner("exporter").SetName("app").SetFullName("exporter/app").
		SetHTMLURL("https://github.com/exporter/app").SetDefaultBranch("main").SetUserID(u.ID).Save(ctx); err != nil {
		t.Fatal(err)
	}

	raw, err := svc.Export(ctx, int64(u.ID))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, secret := range []string{"super-secret-github-token", "secret-hash"} {
		if strings.Contains(s, secret) {
			t.Fatalf("export leaked %q: %s", secret, s)
		}
	}
	var out Export
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Profile.Login != "exporter" || out.Profile.OIDCEmail != "person@example.com" {
		t.Fatalf("unexpected profile: %+v", out.Profile)
	}
	if len(out.Repositories) != 1 || out.Repositories[0].FullName != "exporter/app" {
		t.Fatalf("unexpected repositories: %+v", out.Repositories)
	}
	if len(out.APITokens) != 1 || out.APITokens[0].Name != "ci" {
		t.Fatalf("unexpected tokens: %+v", out.APITokens)
	}
	if out.Preferences.UmamiURL != "https://umami.example.org" {
		t.Fatalf("unexpected preferences: %+v", out.Preferences)
	}
}
