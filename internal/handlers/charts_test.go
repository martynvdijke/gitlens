package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gitlens/ent"
)

func TestParseSince_24h(t *testing.T) {
	since := parseSince("24h")
	if since.IsZero() {
		t.Fatal("expected non-zero time for 24h")
	}
	// Should be approximately 24 hours ago
	if time.Since(since) < 23*time.Hour || time.Since(since) > 25*time.Hour {
		t.Errorf("expected ~24h ago, got %v ago", time.Since(since))
	}
}

func TestParseSince_7d(t *testing.T) {
	since := parseSince("7d")
	if since.IsZero() {
		t.Fatal("expected non-zero time for 7d")
	}
	if time.Since(since) < 6*24*time.Hour || time.Since(since) > 8*24*time.Hour {
		t.Errorf("expected ~7d ago, got %v ago", time.Since(since))
	}
}

func TestParseSince_30d(t *testing.T) {
	since := parseSince("30d")
	if since.IsZero() {
		t.Fatal("expected non-zero time for 30d")
	}
	if time.Since(since) < 29*24*time.Hour || time.Since(since) > 31*24*time.Hour {
		t.Errorf("expected ~30d ago, got %v ago", time.Since(since))
	}
}

func TestParseSince_DefaultNoFilter(t *testing.T) {
	since := parseSince("")
	if !since.IsZero() {
		t.Errorf("expected zero time for empty string, got %v", since)
	}
}

func TestParseSince_InvalidReturnsNoFilter(t *testing.T) {
	since := parseSince("abc")
	if !since.IsZero() {
		t.Errorf("expected zero time for invalid value, got %v", since)
	}
}

func TestParseSince_90d(t *testing.T) {
	since := parseSince("90d")
	if since.IsZero() {
		t.Fatal("expected non-zero time for 90d")
	}
	if time.Since(since) < 89*24*time.Hour || time.Since(since) > 91*24*time.Hour {
		t.Errorf("expected ~90d ago, got %v ago", time.Since(since))
	}
}

func TestDORAChartData_JSONSerialization(t *testing.T) {
	data := DORAChartData{
		Metrics: &DORAMetrics{
			TotalRepos:       3,
			TotalReleases:    10,
			TotalCommits:     200,
			FeatPct:          45.0,
			FixPct:           30.0,
			DocsPct:          15.0,
			ChorePct:         10.0,
			WorkflowSuccesses: 80,
			WorkflowFailures:  20,
			WorkflowPassRate:  80.0,
			AvgLeadTimeHours:  12.5,
			ReleasesPerRepo:   3.3,
		},
		Repos: []RepoChartData{
			{
				FullName:            "user/repo1",
				TotalCommitsFetched: 100,
				WorkflowStatus:      "success",
				ReleaseCount:        5,
				AvgLeadTimeHours:    10.0,
			},
			{
				FullName:            "user/repo2",
				TotalCommitsFetched: 50,
				WorkflowStatus:      "failure",
				ReleaseCount:        3,
				AvgLeadTimeHours:    0,
			},
		},
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal DORAChartData: %v", err)
	}

	raw := string(b)
	if !strings.Contains(raw, "user/repo1") {
		t.Error("expected repo1 in JSON output")
	}
	if !strings.Contains(raw, "workflowPassRate") {
		t.Error("expected workflowPassRate field in JSON")
	}
	if !strings.Contains(raw, "avgLeadTimeHours") {
		t.Error("expected avgLeadTimeHours field in JSON")
	}
	if !strings.Contains(raw, "totalCommitsFetched") {
		t.Error("expected totalCommitsFetched field in JSON")
	}

	// Verify round-trip
	var decoded DORAChartData
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("failed to unmarshal DORAChartData: %v", err)
	}
	if decoded.Metrics.TotalRepos != 3 {
		t.Errorf("expected 3 repos, got %d", decoded.Metrics.TotalRepos)
	}
	if decoded.Repos[0].FullName != "user/repo1" {
		t.Errorf("expected user/repo1, got %s", decoded.Repos[0].FullName)
	}
	if decoded.Repos[1].AvgLeadTimeHours != 0 {
		t.Errorf("expected 0 lead time, got %f", decoded.Repos[1].AvgLeadTimeHours)
	}
}

func TestDORAChartData_MetricsComputedCorrectly(t *testing.T) {
	metrics := computeMetrics([]*ent.Repository{
		{FullName: "a/a", TotalCommitsFetched: 100, FeatCount: 40, FixCount: 30, DocsCount: 20, ChoreCount: 10, OtherCommitCount: 0, WorkflowSuccessCount: 10, WorkflowFailureCount: 2, ReleaseCount: 5, AvgLeadTimeHours: 12.0},
		{FullName: "b/b", TotalCommitsFetched: 50, FeatCount: 10, FixCount: 5, DocsCount: 5, ChoreCount: 0, OtherCommitCount: 0, WorkflowSuccessCount: 5, WorkflowFailureCount: 1, ReleaseCount: 2, AvgLeadTimeHours: 6.0},
	})

	if metrics.TotalRepos != 2 {
		t.Errorf("expected 2 repos, got %d", metrics.TotalRepos)
	}
	if metrics.TotalCommits != 150 {
		t.Errorf("expected 150 commits, got %d", metrics.TotalCommits)
	}
	if metrics.TotalReleases != 7 {
		t.Errorf("expected 7 releases, got %d", metrics.TotalReleases)
	}
	if metrics.FeatPct != 41.7 {
		t.Errorf("expected 41.7%% feat, got %.1f%%", metrics.FeatPct)
	}
	if metrics.FixPct != 29.2 {
		t.Errorf("expected 29.2%% fix, got %.1f%%", metrics.FixPct)
	}
	if metrics.WorkflowPassRate != 83.3 {
		t.Errorf("expected 83.3%% pass rate, got %.1f%%", metrics.WorkflowPassRate)
	}
	if metrics.AvgLeadTimeHours != 9.0 {
		t.Errorf("expected 9.0h avg lead time, got %.1f", metrics.AvgLeadTimeHours)
	}
	if metrics.ReleasesPerRepo != 3.5 {
		t.Errorf("expected 3.5 releases/repo, got %.1f", metrics.ReleasesPerRepo)
	}
}

func TestDORAChartData_EmptyMetrics(t *testing.T) {
	metrics := computeMetrics(nil)
	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}
	if metrics.TotalRepos != 0 {
		t.Errorf("expected 0 repos, got %d", metrics.TotalRepos)
	}
	if metrics.WorkflowPassRate != 0 {
		t.Errorf("expected 0%% pass rate, got %.1f%%", metrics.WorkflowPassRate)
	}
}

func TestDORAChartData_NoWorkflows(t *testing.T) {
	metrics := computeMetrics([]*ent.Repository{
		{FullName: "a/a", TotalCommitsFetched: 10, FeatCount: 5, FixCount: 3, DocsCount: 1, ChoreCount: 1, OtherCommitCount: 0},
	})
	if metrics.WorkflowPassRate != 0 {
		t.Errorf("expected 0%% pass rate when no workflows, got %.1f%%", metrics.WorkflowPassRate)
	}
	if metrics.TotalCommits != 10 {
		t.Errorf("expected 10 commits, got %d", metrics.TotalCommits)
	}
}
