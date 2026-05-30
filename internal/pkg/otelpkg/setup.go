package otelpkg

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// InstallGlobals registers the W3C propagator and optional tracer/meter providers for
// libraries that use the global otel API (e.g. RabbitMQ inject/extract, manual spans).
func InstallGlobals(tracer *OTELTracerProvider, metrics *OTELMetricsProvider) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if tracer != nil {
		otel.SetTracerProvider(tracer.TracerProvider)
	}
	if metrics != nil {
		otel.SetMeterProvider(metrics.MeterProvider)
	}
}
