package otel

import (
	"sync"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	otelTrace "go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware instruments Gin requests with otelgin spans and HTTP server
// metrics. otelgin binds its tracer and meter at construction time, so the
// underlying middleware is rebuilt whenever the global providers change. That
// lets OTEL enabled from the admin panel take effect without a restart.
func HTTPMiddleware(service string) gin.HandlerFunc {
	var (
		mu       sync.Mutex
		cachedTP otelTrace.TracerProvider
		cachedMP metric.MeterProvider
		cached   gin.HandlerFunc
	)

	return func(c *gin.Context) {
		tp := otel.GetTracerProvider()
		mp := otel.GetMeterProvider()

		mu.Lock()
		if cached == nil || tp != cachedTP || mp != cachedMP {
			cachedTP = tp
			cachedMP = mp
			cached = otelgin.Middleware(service,
				otelgin.WithTracerProvider(tp),
				otelgin.WithMeterProvider(mp),
			)
		}
		h := cached
		mu.Unlock()

		h(c)
	}
}
