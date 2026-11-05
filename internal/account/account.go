// Package account owns whole-account operations: exporting a user's data and
// closing the account. Both the HTML app and the JSON API call the same
// service so deletion cannot drift between clients.
package account

import (
	"context"
	stdSQL "database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
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
)

var (
	// ErrLastAdmin means closing this account would leave the instance with no
	// administrator.
	ErrLastAdmin = errors.New("the last admin cannot be deleted")
	// ErrNotFound means there is no account with the requested ID.
	ErrNotFound = errors.New("account not found")
	// ErrInvalid means the request was malformed before it reached the store.
	ErrInvalid = errors.New("invalid account request")
)

// GitHubRevoker asks GitHub to invalidate an OAuth access token.
type GitHubRevoker interface {
	RevokeToken(ctx context.Context, token string) error
}

// ForgejoRevoker asks a specific Forgejo instance to invalidate a token.
type ForgejoRevoker interface {
	RevokeToken(ctx context.Context, instanceBase, token string) error
}

// Service performs whole-account operations.
type Service struct {
	Client         *ent.Client
	DB             *stdSQL.DB
	GitHubRevoker  GitHubRevoker
	ForgejoRevoker ForgejoRevoker
}

// New builds the shared account service. DB is used for the raw sessions table,
// which is outside the Ent schema.
func New(client *ent.Client, db *stdSQL.DB) *Service {
	return &Service{Client: client, DB: db}
}

// Delete closes an account: it revokes provider tokens where possible and then
// removes sessions, API tokens, tracked repositories, repository-derived
// history and the user row in one database transaction. The last admin cannot
// be deleted.
func (s *Service) Delete(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrInvalid
	}
	if s.Client == nil || s.DB == nil {
		return errors.New("account deletion is not configured")
	}

	// Provider revocation is an external HTTP call, so do it before opening the
	// database transaction. A failure is logged but does not block erasure: the
	// stored token is removed either way.
	u, err := s.Client.User.Get(ctx, int(userID))
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	s.revokeProviderTokens(ctx, u)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	txc := ent.NewClient(ent.Driver(entsql.NewDriver(dialect.SQLite, entsql.Conn{ExecQuerier: tx})))

	// Re-read inside the transaction so a concurrent deletion cannot be
	// mistaken for a last-admin refusal.
	current, err := txc.User.Get(ctx, int(userID))
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	if current.IsAdmin {
		admins, err := txc.User.Query().Where(user.IsAdmin(true)).Count(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}

	repoIDs, err := txc.Repository.Query().
		Where(repository.HasUserWith(user.IDEQ(int(userID)))).
		IDs(ctx)
	if err != nil {
		return err
	}
	if len(repoIDs) > 0 {
		if _, err := txc.CommitActivity.Delete().
			Where(commitactivity.RepoIDIn(repoIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := txc.Event.Delete().
			Where(event.RepoIDIn(repoIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := txc.MetricSnapshot.Delete().
			Where(metricsnapshot.RepoIDIn(repoIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := txc.Repository.Delete().
			Where(repository.IDIn(repoIDs...)).Exec(ctx); err != nil {
			return err
		}
	}

	if _, err := txc.ApiToken.Delete().Where(apitoken.UserID(int(userID))).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}

	// The row carries the OIDC subject/email link and all profile data. It is
	// deleted last, after everything that points at it.
	if err := txc.User.DeleteOneID(int(userID)).Exec(ctx); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) revokeProviderTokens(ctx context.Context, u *ent.User) {
	if u.AccessToken != "" && s.GitHubRevoker != nil {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := s.GitHubRevoker.RevokeToken(cctx, u.AccessToken); err != nil {
			log.Printf("account: GitHub token revocation failed for user %d: %v", u.ID, err)
		}
	}
	if u.ForgejoAccessToken != "" && s.ForgejoRevoker != nil {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := s.ForgejoRevoker.RevokeToken(cctx, u.ForgejoURL, u.ForgejoAccessToken); err != nil {
			log.Printf("account: Forgejo token revocation failed for user %d: %v", u.ID, err)
		}
	}
}

// ExportProfile is the account holder's own identity, without credentials.
type ExportProfile struct {
	GithubID     int64     `json:"github_id"`
	Login        string    `json:"login"`
	Name         string    `json:"name,omitempty"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	ForgejoID    int64     `json:"forgejo_id,omitempty"`
	ForgejoLogin string    `json:"forgejo_login,omitempty"`
	ForgejoURL   string    `json:"forgejo_url,omitempty"`
	ForgejoName  string    `json:"forgejo_name,omitempty"`
	OIDCSubject  string    `json:"oidc_subject,omitempty"`
	OIDCEmail    string    `json:"oidc_email,omitempty"`
	Groups       string    `json:"oidc_groups,omitempty"`
	IsAdmin      bool      `json:"is_admin"`
	CreatedAt    time.Time `json:"created_at"`
}

// ExportPreferences is the user-controlled configuration stored on the row.
type ExportPreferences struct {
	SyncIntervalMinutes    int    `json:"sync_interval_minutes"`
	EinkMode               bool   `json:"eink_mode"`
	UmamiURL               string `json:"umami_url,omitempty"`
	UmamiSiteID            string `json:"umami_site_id,omitempty"`
	DismissedForgejoWarned string `json:"dismissed_forgejo_warning_for,omitempty"`
}

// ExportRepository is one tracked repository and its derived metrics. No
// credential or owner user id is included.
type ExportRepository struct {
	ID                   int        `json:"id"`
	Provider             string     `json:"provider"`
	GithubID             int64      `json:"github_id,omitempty"`
	ForgejoID            int64      `json:"forgejo_id,omitempty"`
	Owner                string     `json:"owner"`
	Name                 string     `json:"name"`
	FullName             string     `json:"full_name"`
	Description          string     `json:"description,omitempty"`
	HTMLURL              string     `json:"html_url"`
	Language             string     `json:"language,omitempty"`
	DefaultBranch        string     `json:"default_branch"`
	LatestCommitSHA      string     `json:"latest_commit_sha,omitempty"`
	LatestCommitMessage  string     `json:"latest_commit_message,omitempty"`
	LatestCommitDate     *time.Time `json:"latest_commit_date,omitempty"`
	LatestReleaseTag     string     `json:"latest_release_tag,omitempty"`
	LatestReleaseName    string     `json:"latest_release_name,omitempty"`
	LatestReleaseDate    *time.Time `json:"latest_release_date,omitempty"`
	WorkflowStatus       string     `json:"workflow_status,omitempty"`
	TotalCommitsFetched  int        `json:"total_commits_fetched,omitempty"`
	FeatCount            int        `json:"feat_count,omitempty"`
	FixCount             int        `json:"fix_count,omitempty"`
	DocsCount            int        `json:"docs_count,omitempty"`
	ChoreCount           int        `json:"chore_count,omitempty"`
	OtherCommitCount     int        `json:"other_commit_count,omitempty"`
	ReleaseCount         int        `json:"release_count,omitempty"`
	AvgLeadTimeHours     float64    `json:"avg_lead_time_hours,omitempty"`
	WorkflowSuccessCount int        `json:"workflow_success_count,omitempty"`
	WorkflowFailureCount int        `json:"workflow_failure_count,omitempty"`
	OpenPRCount          int        `json:"open_pr_count,omitempty"`
	PullRequests         string     `json:"pull_requests,omitempty"`
	SyncedAt             *time.Time `json:"synced_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// ExportAPIToken is token metadata only. The secret and hash never leave the
// server.
type ExportAPIToken struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// Export is the portable JSON document returned to the account holder.
type Export struct {
	ExportedAt   time.Time          `json:"exported_at"`
	Profile      ExportProfile      `json:"profile"`
	Preferences  ExportPreferences  `json:"preferences"`
	Repositories []ExportRepository `json:"repositories"`
	APITokens    []ExportAPIToken   `json:"api_tokens"`
}

// Export returns the user's own data as JSON. Token secrets and hashes are
// deliberately absent.
func (s *Service) Export(ctx context.Context, userID int64) ([]byte, error) {
	if userID <= 0 {
		return nil, ErrInvalid
	}
	u, err := s.Client.User.Get(ctx, int(userID))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	repos, err := s.Client.Repository.Query().
		Where(repository.HasUserWith(user.IDEQ(int(userID)))).
		Order(ent.Asc(repository.FieldFullName)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	tokens, err := s.Client.ApiToken.Query().
		Where(apitoken.UserID(int(userID))).
		Order(ent.Desc(apitoken.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := Export{
		ExportedAt: time.Now().UTC(),
		Profile: ExportProfile{
			GithubID:     u.GithubID,
			Login:        u.Login,
			Name:         u.Name,
			AvatarURL:    u.AvatarURL,
			ForgejoID:    u.ForgejoID,
			ForgejoLogin: u.ForgejoLogin,
			ForgejoURL:   u.ForgejoURL,
			ForgejoName:  u.ForgejoName,
			OIDCSubject:  u.OidcSub,
			OIDCEmail:    u.OidcEmail,
			Groups:       u.Groups,
			IsAdmin:      u.IsAdmin,
			CreatedAt:    u.CreatedAt,
		},
		Preferences: ExportPreferences{
			SyncIntervalMinutes:    u.SyncIntervalMinutes,
			EinkMode:               u.EinkMode,
			UmamiURL:               u.UmamiURL,
			UmamiSiteID:            u.UmamiSiteID,
			DismissedForgejoWarned: u.DismissedForgejoWarningFor,
		},
		Repositories: make([]ExportRepository, 0, len(repos)),
		APITokens:    make([]ExportAPIToken, 0, len(tokens)),
	}
	for _, r := range repos {
		out.Repositories = append(out.Repositories, ExportRepository{
			ID:                   r.ID,
			Provider:             r.Provider,
			GithubID:             r.GithubID,
			ForgejoID:            r.ForgejoID,
			Owner:                r.Owner,
			Name:                 r.Name,
			FullName:             r.FullName,
			Description:          r.Description,
			HTMLURL:              r.HTMLURL,
			Language:             r.Language,
			DefaultBranch:        r.DefaultBranch,
			LatestCommitSHA:      r.LatestCommitSha,
			LatestCommitMessage:  r.LatestCommitMessage,
			LatestCommitDate:     timePtr(r.LatestCommitDate),
			LatestReleaseTag:     r.LatestReleaseTag,
			LatestReleaseName:    r.LatestReleaseName,
			LatestReleaseDate:    timePtr(r.LatestReleaseDate),
			WorkflowStatus:       r.WorkflowStatus,
			TotalCommitsFetched:  r.TotalCommitsFetched,
			FeatCount:            r.FeatCount,
			FixCount:             r.FixCount,
			DocsCount:            r.DocsCount,
			ChoreCount:           r.ChoreCount,
			OtherCommitCount:     r.OtherCommitCount,
			ReleaseCount:         r.ReleaseCount,
			AvgLeadTimeHours:     r.AvgLeadTimeHours,
			WorkflowSuccessCount: r.WorkflowSuccessCount,
			WorkflowFailureCount: r.WorkflowFailureCount,
			OpenPRCount:          r.OpenPrCount,
			PullRequests:         r.PullRequests,
			SyncedAt:             timePtr(r.SyncedAt),
			CreatedAt:            r.CreatedAt,
			UpdatedAt:            r.UpdatedAt,
		})
	}
	for _, t := range tokens {
		out.APITokens = append(out.APITokens, ExportAPIToken{
			ID:         t.ID,
			Name:       t.Name,
			CreatedAt:  t.CreatedAt,
			LastUsedAt: t.LastUsedAt,
			ExpiresAt:  t.ExpiresAt,
			RevokedAt:  t.RevokedAt,
		})
	}
	return json.MarshalIndent(out, "", "  ")
}

// ConfirmMatches reports whether the typed confirmation matches the account
// login. Handlers call it before invoking Delete so the service stays free of
// transport concerns.
func ConfirmMatches(login, confirmation string) bool {
	return strings.TrimSpace(confirmation) != "" && strings.EqualFold(strings.TrimSpace(confirmation), strings.TrimSpace(login))
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
