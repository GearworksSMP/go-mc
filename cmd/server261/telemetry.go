package main

import (
	"context"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// initTracer creates a TracerProvider. If OTEL_EXPORTER_OTLP_ENDPOINT is not
// set, a noop provider is returned for zero overhead.
func initTracer(logger *log.Logger) (trace.TracerProvider, func()) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return noop.NewTracerProvider(), func() {}
	}

	ctx := context.Background()
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		logger.Printf("Failed to create OTLP exporter: %v (falling back to noop)", err)
		return noop.NewTracerProvider(), func() {}
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("gearworks-mc"),
		semconv.ServiceVersion("26.1"),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
	)

	logger.Printf("OpenTelemetry tracing enabled (endpoint=%s)", endpoint)
	return tp, func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutCtx); err != nil {
			logger.Printf("Tracer shutdown error: %v", err)
		}
	}
}

// otelSessionEvents implements game.SessionEvents using an OTel span.
type otelSessionEvents struct {
	span trace.Span
}

func (o *otelSessionEvents) OnDeath(deathMessage string) {
	o.span.AddEvent("player.death", trace.WithAttributes(
		attribute.String("death.message", deathMessage),
	))
}

func (o *otelSessionEvents) OnRespawn() {
	o.span.AddEvent("player.respawn")
}

func (o *otelSessionEvents) OnDimensionChange(from, to string) {
	o.span.AddEvent("player.dimension_change", trace.WithAttributes(
		attribute.String("from", from),
		attribute.String("to", to),
	))
}

func (o *otelSessionEvents) OnCommand(command string) {
	o.span.AddEvent("player.command", trace.WithAttributes(
		attribute.String("command", command),
	))
}
