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
	httpRequestsTotal  metric.Int64Counter
	httpRequestDur     metric.Float64Histogram
	metricsOnce        sync.Once
)

// initMetrics instruments creates the OTel metric instruments for HTTP
// request count and duration, using the global meter provider.
func initMetrics() {
	meter := otel.Meter("gitlens")

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
}

// MetricsMiddleware returns a Gin middleware that records HTTP request count
// and duration as OTel metrics. It lazily initializes the instruments on the
// first request.
func MetricsMiddleware() gin.HandlerFunc {
	metricsOnce.Do(initMetrics)

	return func(c *gin.Context) {
		if httpRequestsTotal == nil || httpRequestDur == nil {
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

		httpRequestsTotal.Add(c.Request.Context(), 1, attrs)
		httpRequestDur.Record(c.Request.Context(), elapsed, attrs)
	}
}
