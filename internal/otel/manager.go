package otel

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"gitlens/ent"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
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
		res: resource.NewSchemaless(
			attribute.String("service.name", "gitlens"),
			attribute.String("service.version", "dev"),
		),
	}
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

// Shutdown gracefully shuts down all active providers.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownLocked(ctx)
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

	if cfg.Endpoint == "" {
		return nil
	}
	if !cfg.TracesEnabled && !cfg.MetricsEnabled && !cfg.LogsEnabled {
		return nil
	}

	tp, err := m.buildTracerProvider(ctx, cfg)
	if err != nil {
		log.Printf("otel: build tracer provider: %v", err)
	} else if tp != nil {
		m.tracerProvider = tp
		otel.SetTracerProvider(tp)
	}

	mp, err := m.buildMeterProvider(ctx, cfg)
	if err != nil {
		log.Printf("otel: build meter provider: %v", err)
	} else if mp != nil {
		m.meterProvider = mp
	}

	lp, err := m.buildLoggerProvider(ctx, cfg)
	if err != nil {
		log.Printf("otel: build logger provider: %v", err)
	} else if lp != nil {
		m.loggerProvider = lp
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

func (m *Manager) buildTracerProvider(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	if !cfg.TracesEnabled {
		return nil, nil
	}
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithInsecure(),
	)
	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(m.res),
	), nil
}

func (m *Manager) buildMeterProvider(ctx context.Context, cfg Config) (*sdkmetric.MeterProvider, error) {
	if !cfg.MetricsEnabled {
		return nil, nil
	}
	exp, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(cfg.Endpoint),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	reader := sdkmetric.NewPeriodicReader(exp,
		sdkmetric.WithInterval(30*time.Second),
	)
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(m.res),
	), nil
}

func (m *Manager) buildLoggerProvider(ctx context.Context, cfg Config) (*sdklog.LoggerProvider, error) {
	if !cfg.LogsEnabled {
		return nil, nil
	}
	exp, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(cfg.Endpoint),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	processor := sdklog.NewBatchProcessor(exp)
	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(m.res),
	), nil
}
