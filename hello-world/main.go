package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// version is injected at build time via -ldflags "-X main.version=<version>"
var version = "dev"

type appMetrics struct {
	reqCount    *prometheus.CounterVec
	reqDuration *prometheus.HistogramVec
}

var (
	mtr *appMetrics
)

type dependencyChecker struct {
	db *sql.DB
}

func (c dependencyChecker) pingDatabase(ctx context.Context) error {
	if c.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := c.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	return nil
}

func (c dependencyChecker) readinessHandler(w http.ResponseWriter, r *http.Request) {
	if err := c.pingDatabase(r.Context()); err != nil {
		logger.Warn().Err(err).Msg("readiness check failed")
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// securityHeaders adds standard HTTP security headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

func (c dependencyChecker) livenessHandler(w http.ResponseWriter, r *http.Request) {
	// Liveness probe should only check if the app process is responsive
	// NOT external dependencies. Database issues should affect readiness, not liveness.
	// If we check DB here, database outages will cause Kubernetes to restart healthy pods.
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("alive"))
}

func enableMetrics() *appMetrics {
	mc := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Count of HTTP requests processed, labeled by status and method.",
		},
		[]string{"handler", "method", "status"},
	)
	mh := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Histogram of latencies for HTTP requests.",
		},
		[]string{"handler", "method"},
	)
	prometheus.MustRegister(mc, mh)
	return &appMetrics{reqCount: mc, reqDuration: mh}
}

func getBoolEnv(name string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return def
	}
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Dynamic tracing flag (OpenFeature override-able)
	if isTracingEnabled(ctx) {
		var span trace.Span
		ctx, span = otel.Tracer("hello-world").Start(ctx, "helloHandler")
		defer span.End()
	}

	start := time.Now()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("hello world"))
	dur := time.Since(start).Seconds()
	if isMetricsEnabled(ctx) && mtr != nil {
		mtr.reqCount.WithLabelValues("/", r.Method, "200").Inc()
		mtr.reqDuration.WithLabelValues("/", r.Method).Observe(dur)
	}

	loggerFromContext(ctx).Info().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("remote_addr", r.RemoteAddr).
		Str("user_agent", r.UserAgent()).
		Int("status", http.StatusOK).
		Float64("duration_seconds", dur).
		Msg("handled request")
}

func initTracer(ctx context.Context) (func(context.Context) error, error) {
	// Uses OTEL_EXPORTER_OTLP_ENDPOINT (e.g., http://otel-collector:4318) if set
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp http exporter: %w", err)
	}

	svcName := os.Getenv("OTEL_SERVICE_NAME")
	if svcName == "" {
		svcName = "hello-world"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", svcName),
			attribute.String("service.version", version),
			attribute.String("env", os.Getenv("ENVIRONMENT")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func main() {
	// Initialize structured JSON logger
	initLogger()

	logger.Info().
		Str("version", version).
		Msg("starting hello-world application")

	// Feature flags defaults via env vars
	metricsDefault := getBoolEnv("ENABLE_METRICS", false)
	tracingDefault := getBoolEnv("ENABLE_TRACING", false)
	adminFlagsEnabled := getBoolEnv("ADMIN_FLAGS_ENABLED", false)

	// Initialize OpenFeature (flagd) client for dynamic flags
	initFeatureFlags(tracingDefault, metricsDefault)

	var (
		db    *sql.DB
		err   error
		dbURL = os.Getenv("DATABASE_URL")
	)
	if dbURL != "" {
		db, err = setupDatabase(dbURL)
		if err != nil {
			logger.Fatal().Err(err).Msg("database initialization failed")
		}
		defer func() {
			if cerr := db.Close(); cerr != nil {
				logger.Error().Err(cerr).Msg("database close error")
			}
		}()
	} else {
		logger.Info().Msg("DATABASE_URL not set, skipping database setup")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer shutdownTracerProvider(context.Background())
	if tracingDefault {
		ensureTracerProvider(ctx)
	}

	// Always register metrics collectors; recording/serving is gated dynamically
	mtr = enableMetrics()

	checker := dependencyChecker{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("/", helloHandler)
	mux.HandleFunc("/readyz", checker.readinessHandler)
	mux.HandleFunc("/livez", checker.livenessHandler)

	// Metrics endpoint gated dynamically per-request
	promHandler := promhttp.Handler()
	mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMetricsEnabled(r.Context()) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("metrics disabled"))
			return
		}
		promHandler.ServeHTTP(w, r)
	}))

	// Admin flags (local/dev): GET returns current; POST sets; POST /reset clears overrides
	if adminFlagsEnabled {
		mux.HandleFunc("/admin/flags", adminAuthMiddleware(adminFlagsHandler))
		mux.HandleFunc("/admin/flags/reset", adminAuthMiddleware(adminFlagsResetHandler))
		hasAuth := os.Getenv("ADMIN_API_KEY") != ""
		if hasAuth {
			logger.Info().Msg("Admin flags endpoint enabled with API key authentication: /admin/flags")
		} else {
			logger.Warn().Msg("Admin flags endpoint enabled WITHOUT authentication (dev mode only): /admin/flags")
		}
	}

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	logger.Info().
		Str("addr", addr).
		Bool("admin_flags_enabled", adminFlagsEnabled).
		Msg("server started")

	select {
	case err := <-serverErr:
		if err != nil {
			logger.Fatal().Err(err).Msg("server failed")
		}
	case sig := <-sigCh:
		logger.Info().Str("signal", sig.String()).Msg("received shutdown signal")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("server shutdown error")
		}
		cancel()
		<-serverErr
	}
}

func setupDatabase(databaseURL string) (*sql.DB, error) {
	db, err := waitForDatabase(databaseURL, 45*time.Second)
	if err != nil {
		return nil, err
	}

	// Skip migrations if SKIP_MIGRATIONS=true (they should be run via Kubernetes Job)
	skipMigrations := getBoolEnv("SKIP_MIGRATIONS", false)
	if skipMigrations {
		logger.Info().Msg("SKIP_MIGRATIONS=true, migrations will not run in application")
		return db, nil
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func waitForDatabase(databaseURL string, timeout time.Duration) (*sql.DB, error) {
	deadline := time.Now().Add(timeout)
	for {
		db, err := sql.Open("postgres", databaseURL)
		if err != nil {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("database open failed within deadline: %w", err)
			}
			time.Sleep(2 * time.Second)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr := db.PingContext(ctx)
		cancel()
		if pingErr == nil {
			return db, nil
		}
		db.Close()
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("database not reachable within deadline: %w", pingErr)
		}
		time.Sleep(2 * time.Second)
	}
}

func runMigrations(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file:///migrations", "postgres", driver)
	if err != nil {
		return fmt.Errorf("new migrate: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	if err == migrate.ErrNoChange {
		logger.Info().Msg("migrations: no change")
	} else {
		logger.Info().Msg("migrations: applied successfully")
	}
	return nil
}
