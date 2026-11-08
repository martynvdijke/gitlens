package otel

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	metricsMu         sync.Mutex
	metricsProvider   metric.MeterProvider
	httpRequestsTotal metric.Int64Counter
	httpRequestDur    metric.Float64Histogram
)

// instrumentsFor returns the HTTP metric instruments bound to p, rebuilding
// them when the global meter provider changes (for example after an admin
// reload enables metrics).
func instrumentsFor(p metric.MeterProvider) (metric.Int64Counter, metric.Float64Histogram) {
	metricsMu.Lock()
	defer metricsMu.Unlock()

	if p == metricsProvider && httpRequestsTotal != nil && httpRequestDur != nil {
		return httpRequestsTotal, httpRequestDur
	}
	metricsProvider = p

	meter := p.Meter("gitlens")

	var err error
	httpRequestsTotal, err = meter.Int64Counter(
		"otel_http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		otel.Handle(err)
	}

	httpRequestDur, err = meter.Float64Histogram(
		"otel_http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		otel.Handle(err)
	}

	return httpRequestsTotal, httpRequestDur
}

// MetricsMiddleware returns a Gin middleware that records HTTP request count
// and duration as OTel metrics. Instruments are resolved per request so they
// follow the current meter provider.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		total, dur := instrumentsFor(otel.GetMeterProvider())
		if total == nil || dur == nil {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		elapsed := time.Since(start).Seconds()

		attrs := metric.WithAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.path", c.FullPath()),
			attribute.String("http.status_code", strconv.Itoa(c.Writer.Status())),
		)

		total.Add(c.Request.Context(), 1, attrs)
		dur.Record(c.Request.Context(), elapsed, attrs)
	}
}
