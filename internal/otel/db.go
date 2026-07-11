package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TraceDBQuery wraps a database query function in an OTel span. The span is
// created with the given operation name and any additional attributes. If the
// context does not contain a span, a new root span is created; otherwise the
// span is linked to the parent trace via context propagation.
//
// Usage:
//
//	result, err := TraceDBQuery(ctx, "query_feat_count", func(ctx context.Context) (int, error) {
//	    return client.Repository.Query().Count(ctx)
//	})
func TraceDBQuery[T any](ctx context.Context, operation string, fn func(context.Context) (T, error)) (T, error) {
	tracer := otel.Tracer("gitlens.db")
	ctx, span := tracer.Start(ctx, operation,
		trace.WithAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.system", "sqlite"),
		),
	)
	defer span.End()

	result, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}
