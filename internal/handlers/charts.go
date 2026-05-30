package handlers

import (
	"fmt"
	"html/template"
	"math"
	"net/http"

	"gitlens/ent"
	"gitlens/ent/repository"
	"gitlens/ent/user"

	"github.com/gin-gonic/gin"
)

type ChartHandler struct {
	client *ent.Client
}

func NewChartHandler(client *ent.Client) *ChartHandler {
	return &ChartHandler{client: client}
}

func (h *ChartHandler) Charts(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(c.Request.Context())

	metrics := computeMetrics(repos)

	c.HTML(http.StatusOK, "charts", gin.H{
		"Metrics":        metrics,
		"BarChartSVG":    template.HTML(svgBarChart(metrics)),
		"PieChartSVG":    template.HTML(svgPieChart(metrics)),
		"MetricTrendSVG": template.HTML(svgMetricTrend(metrics)),
	})
}

func svgBarChart(metrics *DORAMetrics) string {
	const width = 300
	const height = 200
	const barHeight = 24
	const barGap = 8
	const leftMargin = 80
	const bottomMargin = 30
	const topMargin = 20

	chartHeight := topMargin + (barHeight+barGap)*5 + bottomMargin

	bars := []struct {
		Label string
		Value float64
		Color string
	}{
		{"Features", metrics.FeatPct, "#3fb950"},
		{"Fixes", metrics.FixPct, "#f85149"},
		{"Docs", metrics.DocsPct, "#d29922"},
		{"Chore", metrics.ChorePct, "#8b949e"},
		{"Other", 100 - metrics.FeatPct - metrics.FixPct - metrics.DocsPct - metrics.ChorePct, "#6e7681"},
	}

	maxVal := 100.0
	usableWidth := width - leftMargin - 20

	s := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#161b22" rx="6"/>
  <text x="%d" y="18" fill="#f0f6fc" font-size="14" font-weight="600">Commit Type Distribution</text>`, width, chartHeight, width, chartHeight, width, chartHeight, width/2)

	for i, b := range bars {
		y := topMargin + i*(barHeight+barGap)
		barW := int(math.Round(b.Value / maxVal * float64(usableWidth)))
		if barW < 0 {
			barW = 0
		}

		s += fmt.Sprintf(`
  <text x="%d" y="%d" fill="#8b949e" font-size="12" text-anchor="end">%s</text>
  <rect x="%d" y="%d" width="%d" height="%d" fill="%s" rx="3"/>
  <text x="%d" y="%d" fill="#e6edf3" font-size="11" dominant-baseline="middle">%.1f%%</text>`,
			leftMargin-6, y+barHeight/2+4,
			b.Label,
			leftMargin, y, barW, barHeight, b.Color,
			leftMargin+8, y+barHeight/2+4,
			b.Value)
	}

	s += "\n</svg>"
	return s
}

func svgPieChart(metrics *DORAMetrics) string {
	const size = 200
	const cx, cy = size / 2, size / 2
	const r = 80
	const innerR = 50

	totalWorkflows := float64(metrics.WorkflowSuccesses + metrics.WorkflowFailures)
	if totalWorkflows == 0 {
		w, h := size, size+40
		return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#161b22" rx="6"/>
  <text x="%d" y="18" fill="#f0f6fc" font-size="14" font-weight="600" text-anchor="middle">Workflow Pass Rate</text>
  <circle cx="%d" cy="%d" r="%d" fill="none" stroke="#21262d" stroke-width="30"/>
  <text x="%d" y="%d" fill="#8b949e" font-size="14" text-anchor="middle">No data</text>
</svg>`, w, h, w, h, w, h, cx, cx, cx, r, cx, cy)
	}

	successAngle := float64(metrics.WorkflowSuccesses) / totalWorkflows * 360
	failAngle := float64(metrics.WorkflowFailures) / totalWorkflows * 360

	successRad := successAngle * math.Pi / 180
	passRate := math.Round(float64(metrics.WorkflowSuccesses)/totalWorkflows*1000) / 10

	x1 := cx + int(float64(r)*math.Sin(successRad))
	y1 := cy - int(float64(r)*math.Cos(successRad))

	largeArc := 0
	if successAngle > 180 {
		largeArc = 1
	}

	path := ""
	if successAngle > 0 && successAngle < 360 {
		path = fmt.Sprintf(`M %d %d L %d %d A %d %d 0 %d 1 %d %d Z`,
			cx, cy, cx, cy-r, r, r, largeArc, x1, y1)
	} else if successAngle >= 360 {
		path = fmt.Sprintf(`M %d %d L %d %d A %d %d 0 1 1 %d %d A %d %d 0 1 1 %d %d Z`,
			cx, cy, cx, cy-r, r, r, cx, cy+r, r, r, cx, cy-r)
	}
	failPath := ""
	if failAngle > 0 {
		failStartX := cx + int(float64(r)*math.Sin(successRad))
		failStartY := cy - int(float64(r)*math.Cos(successRad))
		failEndX := cx
		failEndY := cy - r

		if failAngle >= 360 {
			failPath = fmt.Sprintf(`M %d %d L %d %d A %d %d 0 1 1 %d %d A %d %d 0 1 1 %d %d Z`,
				cx, cy, cx, cy-r, r, r, cx, cy+r, r, r, cx, cy-r)
		} else {
			totalAngle := (successAngle + failAngle) * math.Pi / 180
			failEndX = cx + int(float64(r)*math.Sin(totalAngle))
			failEndY = cy - int(float64(r)*math.Cos(totalAngle))
			failLargeArc := 0
			if failAngle > 180 {
				failLargeArc = 1
			}
			failPath = fmt.Sprintf(`M %d %d L %d %d A %d %d 0 %d 1 %d %d Z`,
				cx, cy, failStartX, failStartY, r, r, failLargeArc, failEndX, failEndY)
		}
	}

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#161b22" rx="6"/>
  <text x="%d" y="18" fill="#f0f6fc" font-size="14" font-weight="600" text-anchor="middle">Workflow Pass Rate</text>
  <circle cx="%d" cy="%d" r="%d" fill="#0d1117" stroke="#21262d" stroke-width="1"/>
  <path d="%s" fill="#3fb950" opacity="0.9"/>
  <path d="%s" fill="#f85149" opacity="0.9"/>
  <circle cx="%d" cy="%d" r="%d" fill="#0d1117"/>
  <text x="%d" y="%d" fill="#f0f6fc" font-size="24" font-weight="700" text-anchor="middle">%.1f%%</text>
  <text x="%d" y="%d" fill="#8b949e" font-size="11" text-anchor="middle">pass rate</text>
</svg>`, size, size+40, size, size+40, size, size+40, size/2, cx, cy, r, path, failPath, cx, cy, innerR, cx, cy-2, passRate, cx, cy+16)
}

func svgMetricTrend(metrics *DORAMetrics) string {
	const width = 300
	const height = 120

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#161b22" rx="6"/>
  <text x="%d" y="18" fill="#f0f6fc" font-size="14" font-weight="600">DORA Metrics Summary</text>
  <text x="16" y="46" fill="#8b949e" font-size="11">Repos Tracked</text>
  <text x="16" y="66" fill="#f0f6fc" font-size="22" font-weight="700">%d</text>
  <text x="120" y="46" fill="#8b949e" font-size="11">Avg Lead Time</text>
  <text x="120" y="66" fill="#f0f6fc" font-size="22" font-weight="700">%.1fh</text>
  <text x="220" y="46" fill="#8b949e" font-size="11">Releases/Repo</text>
  <text x="220" y="66" fill="#f0f6fc" font-size="22" font-weight="700">%.1f</text>
  <text x="16" y="98" fill="#8b949e" font-size="11">Workflow Pass</text>
  <text x="16" y="118" fill="#3fb950" font-size="22" font-weight="700">%.1f%%</text>
  <text x="120" y="98" fill="#8b949e" font-size="11">Total Commits</text>
  <text x="120" y="118" fill="#f0f6fc" font-size="22" font-weight="700">%d</text>
  <text x="220" y="98" fill="#8b949e" font-size="11">Total Releases</text>
  <text x="220" y="118" fill="#f0f6fc" font-size="22" font-weight="700">%d</text>
</svg>`, width, height, width, height, width, height, width/2, metrics.TotalRepos, metrics.AvgLeadTimeHours, metrics.ReleasesPerRepo, metrics.WorkflowPassRate, metrics.TotalCommits, metrics.TotalReleases)
}
