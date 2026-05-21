package handlers

import (
	"fmt"
	"net/http"

	"gitlens/ent"
	"gitlens/ent/repository"

	"github.com/gin-gonic/gin"
)

type BadgeHandler struct {
	client *ent.Client
}

func NewBadgeHandler(client *ent.Client) *BadgeHandler {
	return &BadgeHandler{client: client}
}

func (h *BadgeHandler) Badge(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")

	ctx := c.Request.Context()
	r, err := h.client.Repository.Query().
		Where(
			repository.Owner(owner),
			repository.Name(repo),
		).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		c.Data(http.StatusNotFound, "image/svg+xml", []byte(badgeSVG("unknown", "#6e7681")))
		return
	}

	status := r.WorkflowStatus
	if status == "" || status == "unknown" {
		c.Data(http.StatusOK, "image/svg+xml", []byte(badgeSVG("unknown", "#6e7681")))
		return
	}

	color := "#3fb950"
	label := "passing"
	switch status {
	case "success":
		color = "#3fb950"
		label = "passing"
	case "failure":
		color = "#f85149"
		label = "failing"
	case "cancelled":
		color = "#8b949e"
		label = "cancelled"
	case "neutral":
		color = "#d29922"
		label = "neutral"
	case "skipped":
		color = "#6e7681"
		label = "skipped"
	default:
		color = "#6e7681"
		label = "unknown"
	}

	badge := badgeSVG(label, color)
	c.Data(http.StatusOK, "image/svg+xml", []byte(badge))
}

func badgeSVG(message, color string) string {
	const labelWidth = 70
	const messageWidth = 70
	totalWidth := labelWidth + messageWidth

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="workflow: %s">
  <linearGradient id="s" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r">
    <rect width="%d" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="#555"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">workflow</text>
    <text x="%d" y="14">workflow</text>
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`,
		totalWidth, message,
		totalWidth,
		totalWidth,
		labelWidth, messageWidth, color,
		totalWidth,
		labelWidth/2,
		labelWidth/2,
		labelWidth+messageWidth/2, message,
		labelWidth+messageWidth/2, message,
	)
}
