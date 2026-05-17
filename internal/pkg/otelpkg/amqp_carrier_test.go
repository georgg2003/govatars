package otelpkg_test

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"govatars/internal/pkg/otelpkg"
)

func TestAMQPCarrier_roundTrip(t *testing.T) {
	t.Parallel()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, otelpkg.HeadersCarrier(headers))

	extracted := otel.GetTextMapPropagator().Extract(context.Background(), otelpkg.HeadersCarrier(headers))
	childCtx, childSpan := tp.Tracer("test").Start(extracted, "child")
	childSpan.End()

	require.Equal(t, span.SpanContext().TraceID(), traceIDFrom(childCtx))
}

func traceIDFrom(ctx context.Context) interface{} {
	return trace.SpanFromContext(ctx).SpanContext().TraceID()
}
