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
	"syscall"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"gitoverviewer/ent"
	"gitoverviewer/ent/repository"
	"gitoverviewer/ent/user"
	"gitoverviewer/internal/github"
	"gitoverviewer/internal/handlers"
	"gitoverviewer/internal/middleware"
	"gitoverviewer/internal/sync"
	"gitoverviewer/internal/ws"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "gitoverviewer.db"
	}
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=wal&_fk=1")
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

	sessionStore := middleware.NewSessionStore()

	ghClient := github.NewClient(
		os.Getenv("GITHUB_CLIENT_ID"),
		os.Getenv("GITHUB_CLIENT_SECRET"),
	)

	hub := ws.NewHub()
	go hub.Run()

	syncer := sync.NewSyncer(client, ghClient, hub)

	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
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
	}).ParseFiles(
		"static/index.html",
	))
	syncer.SetTemplate(tmpl)

	authHandler := handlers.NewAuthHandler(client, sessionStore, ghClient)
	dashHandler := handlers.NewDashboardHandler(client, sessionStore, ghClient, syncer)
	settingsHandler := handlers.NewSettingsHandler(client, sessionStore, ghClient, syncer)
	webhookHandler := handlers.NewWebhookHandler(client, syncer, os.Getenv("GITHUB_WEBHOOK_SECRET"))

	r := gin.Default()

	r.SetHTMLTemplate(tmpl)
	r.Static("/static", "./static")

	r.GET("/auth/github", authHandler.Login)
	r.GET("/auth/github/callback", authHandler.Callback)
	r.POST("/auth/logout", authHandler.Logout)

	r.GET("/", dashHandler.Index)

	r.POST("/webhook/github", webhookHandler.HandlePush)

	authed := r.Group("/")
	authed.Use(middleware.AuthRequired(sessionStore))
	{
		authed.GET("/dashboard", dashHandler.Dashboard)
		authed.GET("/repos", dashHandler.ListRepos)
		authed.POST("/repos/:id/sync", dashHandler.SyncRepo)
		authed.POST("/repos/import-all", dashHandler.ImportAllRepos)

		authed.GET("/settings", settingsHandler.Index)
		authed.POST("/settings/interval", settingsHandler.UpdateInterval)
		authed.GET("/settings/repos/available", settingsHandler.AvailableRepos)
		authed.POST("/settings/repos/select", settingsHandler.SelectRepos)
		authed.DELETE("/repos/:id", settingsHandler.RemoveRepo)
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
