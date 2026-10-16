package trace

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartSpan_ProviderSetAfterFirstSpan(t *testing.T) {
	TracingEnabled = true
	prev := otel.GetTracerProvider()
	t.Cleanup(func() {
		TracingEnabled = false
		otel.SetTracerProvider(prev)
	})

	// The cached tracer is created before any provider is set, as when a span starts during startup.
	_, early := StartSpan(t.Context(), "before provider")
	early.End()

	// Each provider set later receives the spans started after it.
	for range 2 {
		rec := tracetest.NewSpanRecorder()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
		for _, name := range []string{"a", "b"} {
			_, span := StartSpan(t.Context(), name)
			span.End()
		}
		ended := rec.Ended()
		require.Equal(t, 2, len(ended))
		require.Equal(t, "a", ended[0].Name())
		require.Equal(t, "b", ended[1].Name())
	}
}

func BenchmarkStartSpan(b *testing.B) {
	TracingEnabled = true
	prev := otel.GetTracerProvider()
	b.Cleanup(func() {
		TracingEnabled = false
		otel.SetTracerProvider(prev)
	})
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.01))))
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, span := StartSpan(b.Context(), "bench")
			span.End()
		}
	})
}
