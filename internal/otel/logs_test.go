package otel

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseLogSeverity(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warning", slog.LevelWarn},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelWarn},
		{"nonsense", slog.LevelWarn},
	}
	for _, tc := range tests {
		if got := ParseLogSeverity(tc.in); got != tc.want {
			t.Errorf("ParseLogSeverity(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestInferLevel(t *testing.T) {
	tests := []struct {
		line string
		want slog.Level
	}{
		{"server started", slog.LevelInfo},
		{"failed to connect to database", slog.LevelError},
		{"sync error: timeout", slog.LevelError},
		{"panic: nil pointer", slog.LevelError},
		{"warning: deprecated option", slog.LevelWarn},
		{"debug: cache miss", slog.LevelDebug},
	}
	for _, tc := range tests {
		if got := inferLevel(tc.line); got != tc.want {
			t.Errorf("inferLevel(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// captureHandler records every slog.Record it receives.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

func TestSeverityHandlerFiltersBelowMinimum(t *testing.T) {
	capture := &captureHandler{}
	logger := slog.New(&severityHandler{next: capture, min: slog.LevelWarn})

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	records := capture.snapshot()
	if len(records) != 2 {
		t.Fatalf("expected 2 records (warn, error), got %d", len(records))
	}
	if records[0].Level != slog.LevelWarn || records[1].Level != slog.LevelError {
		t.Fatalf("unexpected levels: %v, %v", records[0].Level, records[1].Level)
	}
}

func TestBridgeStdLogForwardsLines(t *testing.T) {
	capture := &captureHandler{}
	logger := slog.New(&severityHandler{next: capture, min: slog.LevelDebug})

	restore := BridgeStdLog(logger)
	defer restore()

	log.Printf("something failed: %v", "boom")

	records := capture.snapshot()
	if len(records) != 1 {
		t.Fatalf("expected 1 bridged record, got %d", len(records))
	}
	if records[0].Level != slog.LevelError {
		t.Fatalf("expected error level, got %v", records[0].Level)
	}
	if !strings.Contains(records[0].Message, "something failed: boom") {
		t.Fatalf("unexpected message: %q", records[0].Message)
	}
}

// logCapture collects raw OTLP/HTTP log export bodies.
type logCapture struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *logCapture) add(body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, body)
}

func (c *logCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func startOTLPLogsEndpoint(t *testing.T) (string, *logCapture) {
	t.Helper()
	capture := &logCapture{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.add(body)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), capture
}

func TestReload_LogsEnabledExportsRecords(t *testing.T) {
	setHTTPProtocol(t)
	addr, capture := startOTLPLogsEndpoint(t)

	m, client := newTestManager(t)
	t.Cleanup(func() {
		_ = m.Shutdown(context.Background())
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	})

	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(addr).
		SetTracesEnabled(false).
		SetMetricsEnabled(false).
		SetLogsEnabled(true).
		SetLogSeverity("info").
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m.GetLoggerProvider() == nil {
		t.Fatal("expected a LoggerProvider when logs are enabled")
	}

	slog.Info("integration log record", "signal", "logs")
	log.Printf("integration stdlib failure record")

	if err := m.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if capture.count() == 0 {
		t.Fatal("expected at least one log export to /v1/logs")
	}
}

func TestShutdown_RestoresStdLogOutput(t *testing.T) {
	setHTTPProtocol(t)
	addr, _ := startOTLPLogsEndpoint(t)

	m, client := newTestManager(t)
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(addr).
		SetLogsEnabled(true).
		SetLogSeverity("info").
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if m.GetLoggerProvider() != nil {
		t.Fatal("expected LoggerProvider to be cleared after shutdown")
	}
	// The standard library logger must be usable again after the bridge is
	// removed (it would otherwise still point at the shut-down provider).
	log.Printf("after shutdown")
}
