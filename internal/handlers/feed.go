package handlers

import (
	"net/http"
	"strings"
	"time"

	"gitlens/ent"
	"gitlens/ent/event"
	"gitlens/ent/repository"
	"gitlens/ent/user"

	"github.com/gin-gonic/gin"
)

type FeedHandler struct {
	client *ent.Client
}

func NewFeedHandler(client *ent.Client) *FeedHandler {
	return &FeedHandler{client: client}
}

type feedEvent struct {
	ID        int
	Type      string
	Title     string
	URL       string
	Metadata  string
	Timestamp time.Time
	RepoOwner string
	RepoName  string
	RepoFull  string
}

type feedGroup struct {
	Date   string // "Today", "Yesterday", "May 24"
	Events []feedEvent
}

type feedData struct {
	Groups []feedGroup
	Since  string
	Types  string
	Count  int
}

func (h *FeedHandler) Feed(c *gin.Context) {
	data := h.queryFeed(c)
	c.HTML(http.StatusOK, "feed", data)
}

func (h *FeedHandler) FeedFilter(c *gin.Context) {
	data := h.queryFeed(c)
	c.HTML(http.StatusOK, "feed", data)
}

func (h *FeedHandler) queryFeed(c *gin.Context) *feedData {
	userID := c.GetInt64("user_id")
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		return &feedData{Groups: nil, Since: "7d", Types: "all"}
	}

	since := c.DefaultQuery("since", "7d")
	typesParam := c.DefaultQuery("types", "all")

	if c.Request.Method == "POST" {
		since = c.DefaultPostForm("since", "7d")
		typesParam = c.DefaultPostForm("types", "all")
	}

	var sinceTime time.Time
	switch since {
	case "24h":
		sinceTime = time.Now().Add(-24 * time.Hour)
	case "7d":
		sinceTime = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		sinceTime = time.Now().Add(-30 * 24 * time.Hour)
	default:
		sinceTime = time.Time{} // zero = no filter
	}

	// Build query
	q := h.client.Event.Query().
		Order(ent.Desc(event.FieldTimestamp)).
		Limit(100)

	if !sinceTime.IsZero() {
		q = q.Where(event.TimestampGTE(sinceTime))
	}

	if typesParam != "" && typesParam != "all" {
		typeList := strings.Split(typesParam, ",")
		var typeEnums []event.Type
		for _, t := range typeList {
			switch strings.TrimSpace(t) {
			case "release":
				typeEnums = append(typeEnums, event.TypeRelease)
			case "workflow_failure":
				typeEnums = append(typeEnums, event.TypeWorkflowFailure)
			case "pr_merge":
				typeEnums = append(typeEnums, event.TypePrMerge)
			}
		}
		if len(typeEnums) > 0 {
			q = q.Where(event.TypeIn(typeEnums...))
		}
	}

	dbEvents, err := q.All(c.Request.Context())
	if err != nil || len(dbEvents) == 0 {
		return &feedData{Groups: nil, Since: since, Types: typesParam, Count: 0}
	}

	// Resolve repo names for events
	repoIDs := make([]int, 0, len(dbEvents))
	repoIDSet := make(map[int]bool)
	for _, e := range dbEvents {
		if !repoIDSet[e.RepoID] {
			repoIDSet[e.RepoID] = true
			repoIDs = append(repoIDs, e.RepoID)
		}
	}

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Where(repository.IDIn(repoIDs...)).
		All(c.Request.Context())

	repoNames := make(map[int]struct{ owner, name, full string })
	for _, r := range repos {
		repoNames[r.ID] = struct{ owner, name, full string }{r.Owner, r.Name, r.FullName}
	}

	// Group events by date
	groups := make([]feedGroup, 0)
	var currentGroup *feedGroup
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)

	for _, e := range dbEvents {
		rName := repoNames[e.RepoID]
		dayStart := e.Timestamp.Truncate(24 * time.Hour)
		dateLabel := ""
		switch {
		case dayStart.Equal(today):
			dateLabel = "Today"
		case dayStart.Equal(yesterday):
			dateLabel = "Yesterday"
		default:
			dateLabel = dayStart.Format("Jan 2, 2006")
		}

		if currentGroup == nil || currentGroup.Date != dateLabel {
			if currentGroup != nil {
				groups = append(groups, *currentGroup)
			}
			currentGroup = &feedGroup{Date: dateLabel}
		}

		currentGroup.Events = append(currentGroup.Events, feedEvent{
			ID:        e.ID,
			Type:      string(e.Type),
			Title:     e.Title,
			URL:       e.URL,
			Metadata:  e.Metadata,
			Timestamp: e.Timestamp,
			RepoOwner: rName.owner,
			RepoName:  rName.name,
			RepoFull:  rName.full,
		})
	}
	if currentGroup != nil {
		groups = append(groups, *currentGroup)
	}

	return &feedData{
		Groups: groups,
		Since:  since,
		Types:  typesParam,
		Count:  len(dbEvents),
	}
}
