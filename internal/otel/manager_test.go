package otel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gitlens/ent"
	"gitlens/ent/enttest"

	_ "github.com/mattn/go-sqlite3"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newTestManager(t *testing.T) (*Manager, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	return NewManager(client), client
}

func setHTTPProtocol(t *testing.T) {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
}

func TestNewManager_NoConfig(t *testing.T) {
	m, _ := newTestManager(t)
	defer m.Shutdown(context.Background())

	// With no AdminConfig row, Reload returns empty config → no providers
	if tp := m.GetTracerProvider(); tp == nil {
		t.Fatal("expected non-nil TracerProvider (noop fallback)")
	}
	if mp := m.GetMeterProvider(); mp == nil {
		t.Fatal("expected non-nil MeterProvider (noop fallback)")
	}
	if lp := m.GetLoggerProvider(); lp != nil {
		t.Fatal("expected nil LoggerProvider when logs disabled")
	}
}

func TestReload_EmptyEndpoint(t *testing.T) {
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	// Create AdminConfig with empty endpoint
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint("").
		SetTracesEnabled(true).
		SetMetricsEnabled(true).
		SetLogsEnabled(true).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("expected no error for empty endpoint, got: %v", err)
	}

	// Empty endpoint means no providers should be created
	if tp := m.GetTracerProvider(); tp == nil {
		t.Fatal("expected non-nil TracerProvider (noop fallback)")
	}
	if mp := m.GetMeterProvider(); mp == nil {
		t.Fatal("expected non-nil MeterProvider (noop fallback)")
	}
	if lp := m.GetLoggerProvider(); lp != nil {
		t.Fatal("expected nil LoggerProvider when endpoint empty")
	}
}

func TestReload_AllSignalsDisabled(t *testing.T) {
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	// Create AdminConfig with endpoint but all signals disabled
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint("localhost:4318").
		SetTracesEnabled(false).
		SetMetricsEnabled(false).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("expected no error when all signals disabled, got: %v", err)
	}

	// All signals disabled → no providers
	if tp := m.GetTracerProvider(); tp == nil {
		t.Fatal("expected non-nil TracerProvider (noop fallback)")
	}
	if mp := m.GetMeterProvider(); mp == nil {
		t.Fatal("expected non-nil MeterProvider (noop fallback)")
	}
	if lp := m.GetLoggerProvider(); lp != nil {
		t.Fatal("expected nil LoggerProvider when logs disabled")
	}
}

func startOTLPEndpoint(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	// Return host:port (strip scheme)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestReload_WithTracesAndMetrics(t *testing.T) {
	setHTTPProtocol(t)
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	endpoint := startOTLPEndpoint(t)

	// Create AdminConfig with traces and metrics enabled
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(endpoint).
		SetTracesEnabled(true).
		SetMetricsEnabled(true).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("expected no error reloading with valid config, got: %v", err)
	}

	tp := m.GetTracerProvider()
	if tp == nil {
		t.Fatal("expected non-nil TracerProvider")
	}
	if _, ok := tp.(*sdktrace.TracerProvider); !ok {
		t.Fatal("expected real TracerProvider, not noop")
	}

	mp := m.GetMeterProvider()
	if mp == nil {
		t.Fatal("expected non-nil MeterProvider")
	}

	// Logs disabled → nil logger
	if lp := m.GetLoggerProvider(); lp != nil {
		t.Fatal("expected nil LoggerProvider when logs disabled")
	}
}

func TestReload_ClearsProvidersOnEmptyConfig(t *testing.T) {
	setHTTPProtocol(t)
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	endpoint := startOTLPEndpoint(t)

	// First set up valid config
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(endpoint).
		SetTracesEnabled(true).
		SetMetricsEnabled(true).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("reload with valid config: %v", err)
	}

	if tp := m.GetTracerProvider(); tp == nil {
		t.Fatal("expected TracerProvider after valid reload")
	}

	// Now update to empty config
	_, err = client.AdminConfig.UpdateOneID(1).
		SetOtelEndpoint("").
		SetTracesEnabled(false).
		SetMetricsEnabled(false).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("reload with empty config: %v", err)
	}

	// Providers should be cleared
	if tp := m.GetTracerProvider(); tp == nil {
		t.Fatal("expected non-nil TracerProvider (noop fallback)")
	}
	if mp := m.GetMeterProvider(); mp == nil {
		t.Fatal("expected non-nil MeterProvider (noop fallback)")
	}
	if lp := m.GetLoggerProvider(); lp != nil {
		t.Fatal("expected nil LoggerProvider after clear")
	}
}

func TestReload_MultipleReloads(t *testing.T) {
	setHTTPProtocol(t)
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	endpoint := startOTLPEndpoint(t)

	// Create config
	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(endpoint).
		SetTracesEnabled(true).
		SetMetricsEnabled(true).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Reload twice - should not panic or error
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("first reload: %v", err)
	}
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("second reload: %v", err)
	}

	// Verify providers still work
	tp := m.GetTracerProvider()
	if tp == nil {
		t.Fatal("expected non-nil TracerProvider after multiple reloads")
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	m, _ := newTestManager(t)

	// Shutdown twice should not panic
	ctx := context.Background()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown (idempotent): %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	setHTTPProtocol(t)
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	endpoint := startOTLPEndpoint(t)

	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(endpoint).
		SetTracesEnabled(true).
		SetMetricsEnabled(false).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Reload from multiple goroutines to test mutex safety
	done := make(chan bool)
	for range 5 {
		go func() {
			if err := m.Reload(context.Background()); err != nil {
				t.Logf("concurrent reload error (expected with races): %v", err)
			}
			done <- true
		}()
	}
	for range 5 {
		<-done
	}
}

func TestLoadConfig_HandlesMissingRow(t *testing.T) {
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	// Use the existing manager that loaded from an empty DB
	// Now delete the config if one was created
	client.AdminConfig.Delete().Exec(context.Background())

	cfg, err := m.loadConfig(context.Background())
	if err != nil {
		t.Fatalf("expected no error loading config when no row exists, got: %v", err)
	}
	if cfg.Endpoint != "" {
		t.Fatalf("expected empty endpoint, got: %s", cfg.Endpoint)
	}
	if cfg.TracesEnabled {
		t.Fatal("expected TracesEnabled to be false")
	}
	if cfg.MetricsEnabled {
		t.Fatal("expected MetricsEnabled to be false")
	}
}

func TestReload_OnlyTraces(t *testing.T) {
	setHTTPProtocol(t)
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	endpoint := startOTLPEndpoint(t)

	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(endpoint).
		SetTracesEnabled(true).
		SetMetricsEnabled(false).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if tp := m.GetTracerProvider(); tp == nil {
		t.Fatal("expected TracerProvider")
	} else if _, ok := tp.(*sdktrace.TracerProvider); !ok {
		t.Fatal("expected real TracerProvider")
	}

	if mp := m.GetMeterProvider(); mp == nil {
		t.Fatal("expected non-nil MeterProvider (noop)")
	}
}

func TestReload_OnlyMetrics(t *testing.T) {
	setHTTPProtocol(t)
	m, client := newTestManager(t)
	defer m.Shutdown(context.Background())

	endpoint := startOTLPEndpoint(t)

	_, err := client.AdminConfig.Create().
		SetID(1).
		SetOtelEndpoint(endpoint).
		SetTracesEnabled(false).
		SetMetricsEnabled(true).
		SetLogsEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	mp := m.GetMeterProvider()
	if mp == nil {
		t.Fatal("expected non-nil MeterProvider")
	}

	// Traces should be noop
	if tp := m.GetTracerProvider(); tp == nil {
		t.Fatal("expected non-nil TracerProvider (noop)")
	}
}

func TestNewManager_SetsServiceResource(t *testing.T) {
	m, _ := newTestManager(t)
	defer m.Shutdown(context.Background())

	if m.res == nil {
		t.Fatal("expected non-nil resource")
	}
}

// Test that creating a trace provider with a bad endpoint doesn't crash,
// it just returns nil and logs an error.
func TestBuildTracerProvider_BadEndpoint(t *testing.T) {
	m, _ := newTestManager(t)
	defer m.Shutdown(context.Background())

	// Short timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tp, err := m.buildTracerProvider(ctx, Config{
		Endpoint:      "invalid-endpoint-that-doesnt-exist:9999",
		TracesEnabled: true,
	})
	if err != nil {
		// Error expected - tp should be nil
		if tp != nil {
			t.Fatal("expected nil TracerProvider on error")
		}
		return
	}
	// If by some miracle it didn't error (unlikely with bogus endpoint),
	// make sure we clean up
	if tp != nil {
		tp.Shutdown(ctx)
	}
}

func TestBuildMeterProvider_BadEndpoint(t *testing.T) {
	m, _ := newTestManager(t)
	defer m.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mp, err := m.buildMeterProvider(ctx, Config{
		Endpoint:       "invalid-endpoint-that-doesnt-exist:9999",
		MetricsEnabled: true,
	})
	if err != nil {
		if mp != nil {
			t.Fatal("expected nil MeterProvider on error")
		}
		return
	}
	if mp != nil {
		mp.Shutdown(ctx)
	}
}

// ---- New tests for extended functionality ----

func TestOTLPProtocol_DefaultIsGRPC(t *testing.T) {
	// When OTEL_EXPORTER_OTLP_PROTOCOL is unset, should default to "grpc"
	os.Unsetenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	protocol := otlpProtocol()
	if protocol != "grpc" {
		t.Fatalf("expected default protocol 'grpc', got: %s", protocol)
	}
}

func TestOTLPProtocol_HTTPProtobuf(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	if p := otlpProtocol(); p != "http/protobuf" {
		t.Fatalf("expected 'http/protobuf', got: %s", p)
	}
}

func TestOTLPProtocol_GRPCExplicit(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	if p := otlpProtocol(); p != "grpc" {
		t.Fatalf("expected 'grpc', got: %s", p)
	}
}

func TestSamplerFromEnv_Default(t *testing.T) {
	os.Unsetenv("OTEL_TRACES_SAMPLER")
	os.Unsetenv("OTEL_TRACES_SAMPLER_ARG")
	s := samplerFromEnv()
	if s == nil {
		t.Fatal("expected non-nil sampler")
	}
}

func TestSamplerFromEnv_AlwaysOn(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	s := samplerFromEnv()
	if s == nil {
		t.Fatal("expected non-nil sampler")
	}
}

func TestSamplerFromEnv_AlwaysOff(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")
	s := samplerFromEnv()
	if s == nil {
		t.Fatal("expected non-nil sampler")
	}
}

func TestSamplerFromEnv_TraceIDRatio(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")
	s := samplerFromEnv()
	if s == nil {
		t.Fatal("expected non-nil sampler")
	}
}

func TestSamplerFromEnv_ParentBasedAlwaysOn(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "parentbased_always_on")
	s := samplerFromEnv()
	if s == nil {
		t.Fatal("expected non-nil sampler")
	}
}

func TestBuildResource_DefaultServiceName(t *testing.T) {
	os.Unsetenv("OTEL_SERVICE_NAME")
	os.Unsetenv("OTEL_RESOURCE_ATTRIBUTES")
	m, _ := newTestManager(t)
	defer m.Shutdown(context.Background())

	r := m.buildResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
}

func TestBuildResource_WithEnvServiceName(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "test-service")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=testing")
	m, _ := newTestManager(t)
	defer m.Shutdown(context.Background())

	r := m.buildResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}

	// Verify resource has attributes from env
	var found bool
	for _, attr := range r.Attributes() {
		if attr.Key == attribute.Key("deployment.environment") && attr.Value.AsString() == "testing" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected resource to contain deployment.environment=testing from OTEL_RESOURCE_ATTRIBUTES")
	}
}

func TestShutdownWithTimeout(t *testing.T) {
	m, _ := newTestManager(t)

	// Should not panic or error
	if err := m.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("ShutdownWithTimeout: %v", err)
	}
}

func TestBuildTracerProvider_GRPCBadEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	m, _ := newTestManager(t)
	defer m.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tp, err := m.buildTracerProvider(ctx, Config{
		Endpoint:      "invalid-endpoint:9999",
		TracesEnabled: true,
	})
	if err != nil {
		if tp != nil {
			t.Fatal("expected nil TracerProvider on error")
		}
		return
	}
	if tp != nil {
		tp.Shutdown(ctx)
	}
}

// TestTraceDBQuery_Success verifies the helper runs the function and returns
// its result.
func TestTraceDBQuery_Success(t *testing.T) {
	ctx := context.Background()
	result, err := TraceDBQuery(ctx, "test_query", func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got: %s", result)
	}
}

// TestTraceDBQuery_Error verifies the helper propagates errors.
func TestTraceDBQuery_Error(t *testing.T) {
	ctx := context.Background()
	_, err := TraceDBQuery(ctx, "test_query_err", func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("db error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "db error" {
		t.Fatalf("expected 'db error', got: %v", err)
	}
}
