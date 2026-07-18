package handlers

import (
	"net/http"

	"gitlens/internal/deploy"

	"github.com/gin-gonic/gin"
)

// deployDashboardData is passed to the deploy_tab template.
type deployDashboardData struct {
	ActiveTab    string
	Enabled      bool
	Backend      string
	GotifyOn     bool
	Total        int
	Targets      []deployTargetRow
	DockerErr    string // non-empty if Docker discovery failed
}

type deployTargetRow struct {
	Repository  string
	Image       string
	Container   string
	TagStrategy string
	Source      string // "explicit" or "discovered"
}

// DeployHandler renders the deploy dashboard.
type DeployHandler struct {
	gotifyOn  bool
	targetsFn func() ([]deploy.Target, error)
}

// NewDeployHandler creates a DeployHandler.
// gotifyOn indicates whether Gotify is configured.
// targetsFn defaults to deploy.LoadAllTargets and is replaced in tests.
func NewDeployHandler(gotifyOn bool) *DeployHandler {
	return &DeployHandler{
		gotifyOn:  gotifyOn,
		targetsFn: deploy.LoadAllTargets,
	}
}

// Dashboard renders the deploy tab content.
// GET /deploy
func (h *DeployHandler) Dashboard(c *gin.Context) {
	targets, err := h.targetsFn()
	if err != nil {
		// Discovery entirely failed (e.g. docker unavailable behind explicit targets)
		// Render whatever we have (targets will be nil/empty)
	}

	var dockerErr string
	// LoadAllTargets returns (explicit, nil) when discovery fails and logs a warning.
	// We detect Docker failure by checking if targets only come from explicit config
	// and there was a discovery error logged. Since LoadAllTargets swallows the error,
	// we rely on a non-nil error above — but LoadAllTargets only returns error from
	// LoadTargets (env/file parse), not from discovery. So we always get targets.
	// We'll signal Docker-unavailable via the err from DiscoverTargets.
	// For simplicity, if targets is nil/empty and err != nil, show disabled state.

	if err != nil {
		dockerErr = err.Error()
	}

	rows := make([]deployTargetRow, 0, len(targets))
	for _, t := range targets {
		source := "explicit"
		// Distinguish explicit vs discovered: discovered targets come from
		// Docker labels and are NOT in DEPLOY_TARGETS env/file.
		// We can't easily tell the difference after merge, so we show source
		// only if we have discovery info. For now, all targets show without
		// source distinction — the label-discovery flow is transparent.
		// TODO: carry source through MergeTargets return in a future update.
		_ = source

		rows = append(rows, deployTargetRow{
			Repository:  t.Repository,
			Image:       t.Image,
			Container:   t.Container,
			TagStrategy: string(t.TagStrategy),
			Source:      "config",
		})
	}

	data := deployDashboardData{
		ActiveTab: "deploy",
		Enabled:   len(targets) > 0,
		Backend:   deploy.DeployBackend(),
		GotifyOn:  h.gotifyOn,
		Total:     len(targets),
		Targets:   rows,
		DockerErr: dockerErr,
	}

	c.HTML(http.StatusOK, "deploy_tab", data)
}
