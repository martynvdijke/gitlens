package handlers

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlens/internal/deploy"

	"github.com/gin-gonic/gin"
)

// serveDeployDashboard sets up a gin engine with the deploy_tab template and
// registers the given handler at GET /deploy.
func serveDeployDashboard(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Parse(`{{define "deploy_tab"}}{{end}}`)))
	engine.GET("/deploy", handler)
	req := httptest.NewRequest("GET", "/deploy", nil)
	engine.ServeHTTP(w, req)
	return w
}

func newTestDeployHandler(targets []deploy.Target, err error, gotifyOn bool) *DeployHandler {
	h := NewDeployHandler(gotifyOn)
	h.targetsFn = func() ([]deploy.Target, error) {
		return targets, err
	}
	return h
}

func TestDeployDashboard_WithTargets(t *testing.T) {
	targets := []deploy.Target{
		{Repository: "martynvdijke/gitlens", Image: "ghcr.io/martynvdijke/gitlens", Container: "gitlens", TagStrategy: deploy.TagStrategyReleaseTag},
		{Repository: "org/app", Image: "ghcr.io/org/app", Container: "app-svc", TagStrategy: deploy.TagStrategyLatest},
	}
	h := newTestDeployHandler(targets, nil, true)
	w := serveDeployDashboard(h.Dashboard)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDeployDashboard_NoTargets(t *testing.T) {
	h := newTestDeployHandler(nil, nil, false)
	w := serveDeployDashboard(h.Dashboard)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDeployDashboard_EmptyTargets(t *testing.T) {
	h := newTestDeployHandler([]deploy.Target{}, nil, false)
	w := serveDeployDashboard(h.Dashboard)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDeployDashboard_GotifyOff(t *testing.T) {
	targets := []deploy.Target{
		{Repository: "org/app", Image: "img", Container: "c", TagStrategy: deploy.TagStrategyLatest},
	}
	h := newTestDeployHandler(targets, nil, false)
	w := serveDeployDashboard(h.Dashboard)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDeployDashboard_DockerError(t *testing.T) {
	targets := []deploy.Target{
		{Repository: "org/fallback", Image: "img", Container: "c", TagStrategy: deploy.TagStrategyReleaseTag},
	}
	h := newTestDeployHandler(targets, errors.New("docker not available"), true)
	w := serveDeployDashboard(h.Dashboard)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDeployDashboard_ErrorNoTargets(t *testing.T) {
	h := newTestDeployHandler(nil, errors.New("config failed"), false)
	w := serveDeployDashboard(h.Dashboard)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
