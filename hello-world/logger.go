package main

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

var logger zerolog.Logger

func initLogger() {
	// Configure output format based on environment
	zerolog.TimeFieldFormat = time.RFC3339Nano
	output := os.Stdout

	// Development mode: pretty console output
	// Production mode: JSON
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "pretty" || logFormat == "console" {
		output = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}

	// Set log level from environment (default: info)
	level := zerolog.InfoLevel
	if l := os.Getenv("LOG_LEVEL"); l != "" {
		var err error
		level, err = zerolog.ParseLevel(l)
		if err != nil {
			level = zerolog.InfoLevel
		}
	}

	logger = zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Str("service", "hello-world").
		Str("version", version).
		Logger()
}

// loggerFromContext returns a logger enriched with trace ID if present
func loggerFromContext(ctx context.Context) *zerolog.Logger {
	l := logger.With().Logger()

	// Extract and add trace ID if present
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		l = l.With().
			Str("trace_id", sc.TraceID().String()).
			Str("span_id", sc.SpanID().String()).
			Logger()
	}

	return &l
}
