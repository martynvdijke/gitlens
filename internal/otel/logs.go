package otel

import (
	"bytes"
	"context"
	"io"
	"log"
	"log/slog"
	"strings"
	"sync"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// ParseLogSeverity converts a configured severity name into the minimum
// slog.Level that will be exported. Unknown or empty values fall back to
// slog.LevelWarn, matching the AdminConfig default.
func ParseLogSeverity(severity string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

// severityHandler drops records below min before forwarding them to the
// wrapped handler (the OTel slog bridge). The OTel logs SDK has no built-in
// severity filter, so this is where the admin-configured LogSeverity is
// enforced.
type severityHandler struct {
	next slog.Handler
	min  slog.Level
}

func (h *severityHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.min && h.next.Enabled(ctx, level)
}

func (h *severityHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level < h.min {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *severityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &severityHandler{next: h.next.WithAttrs(attrs), min: h.min}
}

func (h *severityHandler) WithGroup(name string) slog.Handler {
	return &severityHandler{next: h.next.WithGroup(name), min: h.min}
}

// NewHandler returns an slog.Handler that exports records to lp, dropping
// records below the configured severity.
func NewHandler(lp *sdklog.LoggerProvider, severity string) slog.Handler {
	return &severityHandler{
		next: otelslog.NewHandler("gitlens", otelslog.WithLoggerProvider(lp)),
		min:  ParseLogSeverity(severity),
	}
}

// stdLogWriter forwards standard-library log output to a slog.Logger, one
// line per record. The standard library has no severity levels, so the level
// is inferred from the message text.
type stdLogWriter struct {
	logger *slog.Logger
	// also receives the raw output so container/console logs keep working
	// even when the OTel collector is unreachable.
	also io.Writer

	mu  sync.Mutex
	buf []byte
}

func (w *stdLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.also != nil {
		_, _ = w.also.Write(p)
	}

	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		w.emit(line)
	}
	return len(p), nil
}

func (w *stdLogWriter) emit(line string) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return
	}
	w.logger.Log(context.Background(), inferLevel(line), line)
}

// inferLevel maps an unstructured log line to a slog level. This keeps
// error/failure messages visible under the default "warning" threshold while
// still letting informational chatter be filtered out.
func inferLevel(line string) slog.Level {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "error"),
		strings.Contains(l, "failed"),
		strings.Contains(l, "failure"),
		strings.Contains(l, "fatal"),
		strings.Contains(l, "panic"):
		return slog.LevelError
	case strings.Contains(l, "warn"):
		return slog.LevelWarn
	case strings.Contains(l, "debug"):
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// BridgeStdLog routes the standard library default logger through logger so
// existing log.Printf calls are exported via OTel. It returns a function that
// restores the previous output destination.
func BridgeStdLog(logger *slog.Logger) func() {
	previous := log.Writer()
	log.SetOutput(&stdLogWriter{logger: logger, also: previous})
	return func() {
		log.SetOutput(previous)
	}
}
