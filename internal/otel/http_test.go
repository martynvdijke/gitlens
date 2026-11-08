package otel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	otelTrace "go.opentelemetry.io/otel/trace"
)

func TestInstrumentsFor_RebindsOnProviderChange(t *testing.T) {
	p1 := sdkmetric.NewMeterProvider()
	p2 := sdkmetric.NewMeterProvider()

	counter1, hist1 := instrumentsFor(p1)
	counter2, hist2 := instrumentsFor(p2)

	if counter1 == nil || hist1 == nil || counter2 == nil || hist2 == nil {
		t.Fatal("expected non-nil instruments")
	}
	if counter1 == counter2 && hist1 == hist2 {
		t.Fatal("expected instruments to be rebuilt for a new meter provider")
	}

	// Same provider returns the cached instruments.
	counter3, hist3 := instrumentsFor(p2)
	if counter3 != counter2 || hist3 != hist2 {
		t.Fatal("expected cached instruments for the same provider")
	}
}

func TestHTTPMiddleware_RebindsOnProviderChange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	original := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(original) })

	// Start with a noop provider, as the app does when OTEL is not yet
	// configured.
	otel.SetTracerProvider(otelTrace.NewNoopTracerProvider())

	router := gin.New()
	router.Use(HTTPMiddleware("test"))
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	doRequest := func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("unexpected status %d", w.Code)
		}
	}

	doRequest()

	// Enable tracing at runtime and confirm the middleware rebinds.
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	otel.SetTracerProvider(tp)

	doRequest()

	if got := len(exporter.GetSpans()); got == 0 {
		t.Fatal("expected spans after switching to a real tracer provider")
	}
}
