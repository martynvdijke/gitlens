package otel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"gitlens/ent"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	otelTrace "go.opentelemetry.io/otel/trace"
)

// Config holds the parsed OTEL settings from AdminConfig.
type Config struct {
	Endpoint       string
	TracesEnabled  bool
	MetricsEnabled bool
	LogsEnabled    bool
	LogSeverity    string
}

// Manager controls the lifecycle of TracerProvider, MeterProvider, and
// LoggerProvider. Safe for concurrent use.
type Manager struct {
	client *ent.Client
	mu     sync.Mutex

	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider

	res *resource.Resource
}

// NewManager creates a Manager and immediately reloads providers from the
// current AdminConfig. Errors are logged but do not prevent return.
func NewManager(client *ent.Client) *Manager {
	m := &Manager{
		client: client,
	}
	m.res = m.buildResource()
	if err := m.Reload(context.Background()); err != nil {
		log.Printf("otel: initial reload: %v", err)
	}
	return m
}

// Reload reads the AdminConfig from the database and restarts providers.
func (m *Manager) Reload(ctx context.Context) error {
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return err
	}
	return m.reloadFromConfig(ctx, cfg)
}

// Shutdown gracefully shuts down all active providers with a 10s timeout.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownLocked(ctx)
}

// ShutdownWithTimeout shuts down with the given timeout, flushing pending
// spans, metrics, and logs.
func (m *Manager) ShutdownWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return m.Shutdown(ctx)
}

// GetTracerProvider returns the current TracerProvider or a noop fallback.
func (m *Manager) GetTracerProvider() otelTrace.TracerProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tracerProvider != nil {
		return m.tracerProvider
	}
	return otelTrace.NewNoopTracerProvider()
}

// GetMeterProvider returns the current MeterProvider or a noop fallback.
func (m *Manager) GetMeterProvider() *sdkmetric.MeterProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.meterProvider != nil {
		return m.meterProvider
	}
	return sdkmetric.NewMeterProvider()
}

// GetLoggerProvider returns the current LoggerProvider or nil.
func (m *Manager) GetLoggerProvider() *sdklog.LoggerProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loggerProvider
}

// ---- internal ----

func (m *Manager) loadConfig(ctx context.Context) (Config, error) {
	cfg, err := m.client.AdminConfig.Get(ctx, 1)
	if err != nil {
		if ent.IsNotFound(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	return Config{
		Endpoint:       cfg.OtelEndpoint,
		TracesEnabled:  cfg.TracesEnabled,
		MetricsEnabled: cfg.MetricsEnabled,
		LogsEnabled:    cfg.LogsEnabled,
		LogSeverity:    cfg.LogSeverity,
	}, nil
}

func (m *Manager) reloadFromConfig(ctx context.Context, cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.shutdownLocked(ctx); err != nil {
		log.Printf("otel: shutdown during reload: %v", err)
	}

	// Graceful degradation: if no endpoint or all signals disabled, clear
	// providers and return (noop fallback).
	if cfg.Endpoint == "" {
		return nil
	}
	if !cfg.TracesEnabled && !cfg.MetricsEnabled && !cfg.LogsEnabled {
		return nil
	}

	// Try to build tracer provider; on failure log and continue (graceful
	// degradation).
	tp, err := m.buildTracerProvider(ctx, cfg)
	if err != nil {
		log.Printf("otel: build tracer provider (falling back to noop): %v", err)
	} else if tp != nil {
		m.tracerProvider = tp
		otel.SetTracerProvider(tp)
	}

	mp, err := m.buildMeterProvider(ctx, cfg)
	if err != nil {
		log.Printf("otel: build meter provider (falling back to noop): %v", err)
	} else if mp != nil {
		m.meterProvider = mp
		otel.SetMeterProvider(mp)
	}

	lp, err := m.buildLoggerProvider(ctx, cfg)
	if err != nil {
		log.Printf("otel: build logger provider (falling back to noop): %v", err)
	} else if lp != nil {
		m.loggerProvider = lp
		// Wire the slog bridge so slog log records flow through OTel.
		global.SetLoggerProvider(lp)
		slog.SetDefault(otelslog.NewLogger("gitlens", otelslog.WithLoggerProvider(lp)))
	}

	return nil
}

func (m *Manager) shutdownLocked(ctx context.Context) error {
	var errs []error

	if m.tracerProvider != nil {
		if err := m.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		m.tracerProvider = nil
	}
	if m.meterProvider != nil {
		if err := m.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		m.meterProvider = nil
	}
	if m.loggerProvider != nil {
		if err := m.loggerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		m.loggerProvider = nil
	}

	return errors.Join(errs...)
}

// otlpProtocol returns the OTLP protocol from the environment, defaulting to
// "grpc". Supports "grpc" and "http/protobuf".
func otlpProtocol() string {
	p := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	if p == "" {
		return "grpc"
	}
	return strings.ToLower(p)
}

// samplerFromEnv returns an sdktrace.Sampler configured from
// OTEL_TRACES_SAMPLER and OTEL_TRACES_SAMPLER_ARG env vars following the OTel
// specification.
func samplerFromEnv() sdktrace.Sampler {
	sampler := os.Getenv("OTEL_TRACES_SAMPLER")
	arg := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	switch strings.ToLower(sampler) {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(parseSamplerArg(arg, 1.0))
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseSamplerArg(arg, 1.0)))
	default:
		// Default: parentbased_always_on (OTel spec default)
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func parseSamplerArg(arg string, defaultVal float64) float64 {
	if arg == "" {
		return defaultVal
	}
	// Try to parse as float64
	var f float64
	if _, err := fmt.Sscanf(arg, "%f", &f); err != nil {
		return defaultVal
	}
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// buildResource creates a resource with attributes from OTEL_RESOURCE_ATTRIBUTES
// and OTEL_SERVICE_NAME env vars, combined with the static service info.
func (m *Manager) buildResource() *resource.Resource {
	svcName := os.Getenv("OTEL_SERVICE_NAME")
	if svcName == "" {
		svcName = "gitlens"
	}

	r := resource.NewSchemaless(
		attribute.String("service.name", svcName),
		attribute.String("service.version", "dev"),
	)

	// Merge with env-var-based resource attributes (OTEL_RESOURCE_ATTRIBUTES
	// and OTEL_SERVICE_NAME). WithFromEnv reads these env vars and produces a
	// resource. Merge with our static resource, giving precedence to env-based
	// attributes.
	envRes, _ := resource.New(context.Background(),
		resource.WithFromEnv(),
	)
	r, _ = resource.Merge(envRes, r)

	return r
}

func (m *Manager) buildTracerProvider(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	if !cfg.TracesEnabled {
		return nil, nil
	}

	var exp sdktrace.SpanExporter
	var err error

	protocol := otlpProtocol()
	switch protocol {
	case "http/protobuf":
		client := otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint(cfg.Endpoint),
			otlptracehttp.WithInsecure(),
		)
		exp, err = otlptrace.New(ctx, client)
	default: // "grpc" (default)
		exp, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
			otlptracegrpc.WithInsecure(),
		)
	}
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(m.res),
		sdktrace.WithSampler(samplerFromEnv()),
	), nil
}

func (m *Manager) buildMeterProvider(ctx context.Context, cfg Config) (*sdkmetric.MeterProvider, error) {
	if !cfg.MetricsEnabled {
		return nil, nil
	}

	var reader sdkmetric.Reader
	protocol := otlpProtocol()
	switch protocol {
	case "http/protobuf":
		exp, err := otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithEndpoint(cfg.Endpoint),
			otlpmetrichttp.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
		reader = sdkmetric.NewPeriodicReader(exp,
			sdkmetric.WithInterval(30*time.Second),
		)
	default: // "grpc" (default)
		exp, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
		reader = sdkmetric.NewPeriodicReader(exp,
			sdkmetric.WithInterval(30*time.Second),
		)
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(m.res),
	), nil
}

func (m *Manager) buildLoggerProvider(ctx context.Context, cfg Config) (*sdklog.LoggerProvider, error) {
	if !cfg.LogsEnabled {
		return nil, nil
	}

	var processorOpt sdklog.LoggerProviderOption
	protocol := otlpProtocol()
	switch protocol {
	case "http/protobuf":
		exp, err := otlploghttp.New(ctx,
			otlploghttp.WithEndpoint(cfg.Endpoint),
			otlploghttp.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
		processorOpt = sdklog.WithProcessor(sdklog.NewBatchProcessor(exp))
	default: // "grpc" (default)
		exp, err := otlploggrpc.New(ctx,
			otlploggrpc.WithEndpoint(cfg.Endpoint),
			otlploggrpc.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
		processorOpt = sdklog.WithProcessor(sdklog.NewBatchProcessor(exp))
	}

	return sdklog.NewLoggerProvider(
		processorOpt,
		sdklog.WithResource(m.res),
	), nil
}
