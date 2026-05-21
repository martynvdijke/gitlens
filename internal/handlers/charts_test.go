package handlers

import (
	"strings"
	"testing"
)

func TestSVGBarChart(t *testing.T) {
	metrics := &DORAMetrics{
		TotalRepos: 3,
		FeatPct:    40.0,
		FixPct:     30.0,
		DocsPct:    20.0,
		ChorePct:   10.0,
	}

	svg := svgBarChart(metrics)
	if !strings.HasPrefix(svg, `<svg`) {
		t.Errorf("expected SVG output, got: %s", svg[:50])
	}
	if !strings.Contains(svg, "Features") {
		t.Errorf("expected Features label, got: %s", svg)
	}
	if !strings.Contains(svg, "40.0%") {
		t.Errorf("expected 40.0%%, got: %s", svg)
	}
	if !strings.Contains(svg, "30.0%") {
		t.Errorf("expected 30.0%%, got: %s", svg)
	}
	if !strings.Contains(svg, "#3fb950") {
		t.Errorf("expected green color for features")
	}
	if !strings.Contains(svg, "#f85149") {
		t.Errorf("expected red color for fixes")
	}
}

func TestSVGBarChart_ZeroValues(t *testing.T) {
	metrics := &DORAMetrics{
		TotalRepos: 1,
		FeatPct:    0,
		FixPct:     0,
		DocsPct:    0,
		ChorePct:   0,
	}

	svg := svgBarChart(metrics)
	if !strings.HasPrefix(svg, `<svg`) {
		t.Errorf("expected SVG output")
	}
}

func TestSVGPieChart(t *testing.T) {
	metrics := &DORAMetrics{
		WorkflowSuccesses: 80,
		WorkflowFailures:  20,
	}

	svg := svgPieChart(metrics)
	if !strings.HasPrefix(svg, `<svg`) {
		t.Errorf("expected SVG output, got: %s", svg[:50])
	}
	if !strings.Contains(svg, "80.0%") {
		t.Errorf("expected 80.0%%, got: %s", svg)
	}
	if !strings.Contains(svg, "#3fb950") {
		t.Errorf("expected green for success")
	}
}

func TestSVGPieChart_Empty(t *testing.T) {
	metrics := &DORAMetrics{}
	svg := svgPieChart(metrics)
	if !strings.Contains(svg, "No data") {
		t.Errorf("expected 'No data' for empty workflows, got: %s", svg)
	}
}

func TestSVGPieChart_AllSuccess(t *testing.T) {
	metrics := &DORAMetrics{
		WorkflowSuccesses: 100,
		WorkflowFailures:  0,
	}

	svg := svgPieChart(metrics)
	if !strings.Contains(svg, "100.0%") {
		t.Errorf("expected 100.0%%, got: %s", svg)
	}
}

func TestSVGPieChart_AllFailures(t *testing.T) {
	metrics := &DORAMetrics{
		WorkflowSuccesses: 0,
		WorkflowFailures:  50,
	}

	svg := svgPieChart(metrics)
	if !strings.Contains(svg, "0.0%") {
		t.Errorf("expected 0.0%%, got: %s", svg)
	}
}

func TestSVGMetricTrend(t *testing.T) {
	metrics := &DORAMetrics{
		TotalRepos:        5,
		TotalReleases:     20,
		TotalCommits:      500,
		AvgLeadTimeHours:  12.5,
		ReleasesPerRepo:   4.0,
		WorkflowPassRate:  95.0,
	}

	svg := svgMetricTrend(metrics)
	if !strings.HasPrefix(svg, `<svg`) {
		t.Errorf("expected SVG output, got: %s", svg[:50])
	}
	if !strings.Contains(svg, "5") || !strings.Contains(svg, "12.5h") {
		t.Errorf("expected metric values in output")
	}
	if !strings.Contains(svg, "95.0%") {
		t.Errorf("expected pass rate")
	}
}

func TestSVGChartsIntegrity(t *testing.T) {
	metrics := &DORAMetrics{
		TotalRepos:        2,
		TotalReleases:     8,
		TotalCommits:      120,
		FeatCount:         30,
		FixCount:          20,
		DocsCount:         10,
		ChoreCount:        5,
		OtherCount:        5,
		FeatPct:           50.0,
		FixPct:            33.3,
		DocsPct:           16.7,
		ChorePct:          0.0,
		WorkflowSuccesses: 25,
		WorkflowFailures:  5,
		WorkflowPassRate:  83.3,
		AvgLeadTimeHours:  48.0,
		ReleasesPerRepo:   4.0,
	}

	bar := svgBarChart(metrics)
	pie := svgPieChart(metrics)
	trend := svgMetricTrend(metrics)

	if !strings.Contains(bar, "50.0%") {
		t.Errorf("bar chart missing 50.0%%")
	}
	if !strings.Contains(pie, "83.3%") {
		t.Errorf("pie chart missing 83.3%%")
	}
	if !strings.Contains(trend, "48.0") {
		t.Errorf("trend chart missing 48.0")
	}
}
