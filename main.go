package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"gitlens/ent"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/github"
	"gitlens/internal/handlers"
	"gitlens/internal/middleware"
	"gitlens/internal/sync"
	"gitlens/internal/ws"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

var Version = "dev" // overridden by ldflags at build time

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "gitlens.db"
	}
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_journal_mode=wal&_fk=1")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	if err := client.Schema.Create(context.Background()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	sessionStore := middleware.NewSessionStore(db)

	ghClient := github.NewClient(
		os.Getenv("GITHUB_CLIENT_ID"),
		os.Getenv("GITHUB_CLIENT_SECRET"),
	)

	hub := ws.NewHub()
	go hub.Run()

	syncer := sync.NewSyncer(client, ghClient, hub)

	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"appVersion": func() string { return Version },
		"shortSHA": func(s string) string {
			if len(s) > 7 {
				return s[:7]
			}
			return s
		},
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("Jan 2, 2006 15:04")
		},
		"truncate": func(s string, n int) string {
			runes := []rune(s)
			if len(runes) > n {
				return string(runes[:n]) + "..."
			}
			return s
		},
		"workflowIcon": func(status string) string {
			switch status {
			case "success":
				return "\u2705"
			case "failure":
				return "\u274C"
			case "neutral", "skipped":
				return "\u23FA"
			default:
				return "\u2753"
			}
		},
		"workflowLabel": func(status string) string {
			switch status {
			case "success":
				return "Passing"
			case "failure":
				return "Failing"
			case "neutral":
				return "Neutral"
			case "skipped":
				return "Skipped"
			case "cancelled":
				return "Cancelled"
			default:
				return "Unknown"
			}
		},
		"hasWorkflowRun": func(status string) bool {
			return status != "" && status != "unknown"
		},
		"printf": func(format string, args ...interface{}) string {
			return fmt.Sprintf(format, args...)
		},
		"releaseIcon": func(conclusion string) string {
			switch conclusion {
			case "success":
				return "\u2705"
			case "failure":
				return "\u274C"
			default:
				return "\u2753"
			}
		},
		"releaseLabel": func(conclusion string) string {
			switch conclusion {
			case "success":
				return "Passing"
			case "failure":
				return "Failing"
			default:
				return "Unknown"
			}
		},
		"hasReleaseConclusion": func(s string) bool {
			return s != "" && s != "unknown"
		},
		"contains": func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		"timeSince": func(t time.Time) string {
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				m := int(d.Minutes())
				if m == 1 { return "1 minute ago" }
				return fmt.Sprintf("%d minutes ago", m)
			case d < 24*time.Hour:
				h := int(d.Hours())
				if h == 1 { return "1 hour ago" }
				return fmt.Sprintf("%d hours ago", h)
			default:
				days := int(d.Hours() / 24)
				if days == 1 { return "1 day ago" }
				return fmt.Sprintf("%d days ago", days)
			}
		},
		"eventIcon": func(eventType string) string {
			switch eventType {
			case "release":
				return "<svg width=\"16\" height=\"16\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"#3fb950\" stroke-width=\"2\"><polyline points=\"23 6 13.5 15.5 8.5 10.5 1 18\"/><polyline points=\"17 6 23 6 23 12\"/></svg>"
			case "workflow_failure":
				return "<svg width=\"16\" height=\"16\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"#f85149\" stroke-width=\"2\"><circle cx=\"12\" cy=\"12\" r=\"10\"/><line x1=\"15\" y1=\"9\" x2=\"9\" y2=\"15\"/><line x1=\"9\" y1=\"9\" x2=\"15\" y2=\"15\"/></svg>"
			case "pr_merge":
				return "<svg width=\"16\" height=\"16\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"#58a6ff\" stroke-width=\"2\"><circle cx=\"18\" cy=\"18\" r=\"3\"/><circle cx=\"6\" cy=\"6\" r=\"3\"/><path d=\"M6 21V9c0 2 2 3 6 3s6 1 6 3v3\"/></svg>"
			default:
				return ""
			}
		},
	}).ParseFiles(
		"static/index.html",
	))
	syncer.SetTemplate(tmpl)

	authHandler := handlers.NewAuthHandler(client, sessionStore, ghClient)
	dashHandler := handlers.NewDashboardHandler(client, sessionStore, ghClient, syncer)
	settingsHandler := handlers.NewSettingsHandler(client, sessionStore, ghClient, syncer)
	chartHandler := handlers.NewChartHandler(client)
	badgeHandler := handlers.NewBadgeHandler(client)
	gitHubAppHandler := handlers.NewGitHubAppHandler(client)
	webhookHandler := handlers.NewWebhookHandler(client, syncer, os.Getenv("GITHUB_WEBHOOK_SECRET"))
	feedHandler := handlers.NewFeedHandler(client)

	r := gin.Default()

	r.SetHTMLTemplate(tmpl)
	r.Static("/static", "./static")

	r.GET("/auth/github", authHandler.Login)
	r.GET("/auth/github/callback", authHandler.Callback)
	r.POST("/auth/logout", authHandler.Logout)

	r.GET("/", dashHandler.Index)

	r.GET("/badge/:owner/:repo", badgeHandler.Badge)
	r.POST("/webhook/github", webhookHandler.HandlePush)
	r.POST("/webhook/github-app", gitHubAppHandler.HandleInstallation)

	authed := r.Group("/")
	authed.Use(middleware.AuthRequired(sessionStore))
	{
		authed.GET("/dashboard", dashHandler.Dashboard)
		authed.GET("/repos", dashHandler.ListRepos)
		authed.POST("/repos/:id/sync", dashHandler.SyncRepo)
		authed.POST("/repos/import-all", dashHandler.ImportAllRepos)
		authed.GET("/repos/:id/prs", dashHandler.ListPullRequests)
		authed.POST("/repos/:id/prs/:number/merge", dashHandler.MergePR)
		authed.POST("/repos/:id/prs/merge-all", dashHandler.MergeAllPRs)

		authed.GET("/settings", settingsHandler.Index)
		authed.POST("/settings/interval", settingsHandler.UpdateInterval)
		authed.GET("/settings/repos/available", settingsHandler.AvailableRepos)
		authed.POST("/settings/repos/select", settingsHandler.SelectRepos)
		authed.DELETE("/repos/:id", settingsHandler.RemoveRepo)

		authed.GET("/charts", chartHandler.Charts)
		authed.GET("/feed", feedHandler.Feed)
		authed.POST("/feed/filter", feedHandler.FeedFilter)
		authed.POST("/repos/setup-webhooks", gitHubAppHandler.SetupAutoWebhooks)
	}

	r.GET("/ws", func(c *gin.Context) {
		hub.HandleWebSocket(c.Writer, c.Request)
	})

	_ = repository.FieldName
	_ = user.FieldLogin

	port := os.Getenv("PORT")
	if port == "" {
		port = "6270"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go syncer.StartPeriodicSync(ctx)

	go func() {
		log.Printf("Server starting on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
}
