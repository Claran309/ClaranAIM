// Package observability wires local Prometheus metrics and OpenTelemetry exporters.
package observability

import (
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/logger"
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Runtime describes one service observability runtime.
type Runtime struct {
	Enabled        bool
	ServiceName    string
	MetricsAddress string
	server         *http.Server
	listener       net.Listener
	registry       *prometheus.Registry
	tracerProvider *sdktrace.TracerProvider
	metricProvider *sdkmetric.MeterProvider
	tracer         trace.Tracer
	otelMetrics    *otelInstruments
	startedAt      time.Time
}

type otelInstruments struct {
	serviceStart     metric.Int64Counter
	dependencyCheck  metric.Int64Counter
	httpRequests     metric.Int64Counter
	httpDurationMS   metric.Float64Histogram
	rpcRequests      metric.Int64Counter
	rpcDurationMS    metric.Float64Histogram
	businessEvents   metric.Int64Counter
	businessDuration metric.Float64Histogram
	businessGauge    metric.Float64Gauge
}

var (
	globalMu      sync.RWMutex
	globalRuntime *Runtime

	serviceStartTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "claran_service_start_total",
		Help: "ClaranAIM service starts.",
	}, []string{"service", "environment"})
	serviceUptimeSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "claran_service_uptime_seconds",
		Help: "ClaranAIM service uptime in seconds.",
	}, []string{"service"})
	dependencyCheckTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "claran_dependency_check_total",
		Help: "Dependency check results by dependency and status.",
	}, []string{"service", "dependency", "status"})
	httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "claran_http_requests_total",
		Help: "HTTP requests by route, method and status.",
	}, []string{"service", "method", "route", "status"})
	httpDurationMS = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "claran_http_duration_ms",
		Help:    "HTTP request duration in milliseconds.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
	}, []string{"service", "method", "route", "status"})
	rpcRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "claran_rpc_requests_total",
		Help: "Kitex RPC requests by service, method and status.",
	}, []string{"service", "method", "status"})
	rpcDurationMS = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "claran_rpc_duration_ms",
		Help:    "Kitex RPC duration in milliseconds.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000},
	}, []string{"service", "method", "status"})
	businessEventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "claran_business_events_total",
		Help: "Domain-level business events by operation and status.",
	}, []string{"service", "domain", "operation", "status"})
	businessDurationMS = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "claran_business_duration_ms",
		Help:    "Domain-level business operation duration in milliseconds.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000},
	}, []string{"service", "domain", "operation", "status"})
	businessGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "claran_business_gauge",
		Help: "Domain-level business gauge values.",
	}, []string{"service", "domain", "name"})
)

// Init starts local /metrics endpoint and configures OTLP trace exporting when enabled.
func Init(serviceName string, cfg config.ObservabilityConfig) (*Runtime, error) {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "app"
	}
	if !cfg.Enabled {
		runtime := &Runtime{Enabled: false, ServiceName: serviceName}
		setGlobal(runtime)
		return runtime, nil
	}
	address := strings.TrimSpace(cfg.MetricsAddress)
	if address == "" {
		address = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	registry := prometheus.NewRegistry()
	for _, collector := range []prometheus.Collector{
		serviceStartTotal,
		serviceUptimeSeconds,
		dependencyCheckTotal,
		httpRequestsTotal,
		httpDurationMS,
		rpcRequestsTotal,
		rpcDurationMS,
		businessEventsTotal,
		businessDurationMS,
		businessGauge,
	} {
		_ = registry.Register(collector)
	}
	runtime := &Runtime{
		Enabled:        true,
		ServiceName:    serviceName,
		MetricsAddress: ln.Addr().String(),
		server: &http.Server{
			Handler: metricsHandler(registry),
		},
		listener:  ln,
		registry:  registry,
		startedAt: time.Now(),
	}
	if tp, tracer, err := initTracing(context.Background(), serviceName, cfg); err != nil {
		logger.Warn("OpenTelemetry trace exporter初始化失败，已仅启用本地Prometheus指标", "service", serviceName, "error", err)
	} else {
		runtime.tracerProvider = tp
		runtime.tracer = tracer
	}
	if mp, instruments, err := initOTLPMetrics(context.Background(), serviceName, cfg); err != nil {
		logger.Warn("OpenTelemetry metrics exporter初始化失败，已仅启用本地Prometheus指标", "service", serviceName, "error", err)
	} else {
		runtime.metricProvider = mp
		runtime.otelMetrics = instruments
	}
	setGlobal(runtime)
	environment := strings.TrimSpace(cfg.Environment)
	if environment == "" {
		environment = "dev"
	}
	serviceStartTotal.WithLabelValues(serviceName, environment).Inc()
	if runtime.otelMetrics != nil {
		runtime.otelMetrics.serviceStart.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("service", serviceName),
			attribute.String("environment", environment),
		))
	}
	serviceUptimeSeconds.WithLabelValues(serviceName).Set(0)
	go runtime.serve()
	logger.Info("可观测性指标端点已启动", "service", serviceName, "metrics", runtime.MetricsAddress, "otlp_endpoint", cfg.OTLPEndpoint)
	return runtime, nil
}

func initTracing(ctx context.Context, serviceName string, cfg config.ObservabilityConfig) (*sdktrace.TracerProvider, trace.Tracer, error) {
	endpoint := normalizeOTLPEndpoint(cfg.OTLPEndpoint, "/v1/traces")
	if endpoint == "" {
		return nil, nil, nil
	}
	clientOptions := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(endpoint)}
	if strings.HasPrefix(endpoint, "http://") {
		clientOptions = append(clientOptions, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, clientOptions...)
	if err != nil {
		return nil, nil, err
	}
	res, err := observabilityResource(serviceName, cfg.Environment)
	if err != nil {
		return nil, nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, tp.Tracer("ClaranAIM/" + serviceName), nil
}

func initOTLPMetrics(ctx context.Context, serviceName string, cfg config.ObservabilityConfig) (*sdkmetric.MeterProvider, *otelInstruments, error) {
	endpoint := normalizeOTLPEndpoint(cfg.OTLPEndpoint, "/v1/metrics")
	if endpoint == "" {
		return nil, nil, nil
	}
	clientOptions := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpointURL(endpoint)}
	if strings.HasPrefix(endpoint, "http://") {
		clientOptions = append(clientOptions, otlpmetrichttp.WithInsecure())
	}
	exporter, err := otlpmetrichttp.New(ctx, clientOptions...)
	if err != nil {
		return nil, nil, err
	}
	res, err := observabilityResource(serviceName, cfg.Environment)
	if err != nil {
		return nil, nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithResource(res),
	)
	meter := mp.Meter("ClaranAIM/" + serviceName)
	instruments := &otelInstruments{}
	if instruments.serviceStart, err = meter.Int64Counter("claran_service_start_total"); err != nil {
		return nil, nil, err
	}
	if instruments.dependencyCheck, err = meter.Int64Counter("claran_dependency_check_total"); err != nil {
		return nil, nil, err
	}
	if instruments.httpRequests, err = meter.Int64Counter("claran_http_requests_total"); err != nil {
		return nil, nil, err
	}
	if instruments.httpDurationMS, err = meter.Float64Histogram("claran_http_duration_ms"); err != nil {
		return nil, nil, err
	}
	if instruments.rpcRequests, err = meter.Int64Counter("claran_rpc_requests_total"); err != nil {
		return nil, nil, err
	}
	if instruments.rpcDurationMS, err = meter.Float64Histogram("claran_rpc_duration_ms"); err != nil {
		return nil, nil, err
	}
	if instruments.businessEvents, err = meter.Int64Counter("claran_business_events_total"); err != nil {
		return nil, nil, err
	}
	if instruments.businessDuration, err = meter.Float64Histogram("claran_business_duration_ms"); err != nil {
		return nil, nil, err
	}
	if instruments.businessGauge, err = meter.Float64Gauge("claran_business_gauge"); err != nil {
		return nil, nil, err
	}
	return mp, instruments, nil
}

func observabilityResource(serviceName, environment string) (*resource.Resource, error) {
	environment = strings.TrimSpace(environment)
	if environment == "" {
		environment = "dev"
	}
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("deployment.environment", environment),
		),
	)
}

func normalizeOTLPEndpoint(raw, defaultPath string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	if shouldUseDefaultOTLPPath(parsed.Path, defaultPath) {
		parsed.Path = defaultPath
	}
	return parsed.String()
}

func shouldUseDefaultOTLPPath(path, defaultPath string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return true
	}
	if path == defaultPath {
		return false
	}
	switch path {
	case "/v1/traces", "/v1/metrics", "/v1/logs":
		return true
	default:
		return false
	}
}

// InitService initializes observability for a service and returns a shutdown callback.
// Startup should not fail merely because the local observability stack is down.
func InitService(serviceName string, cfg *config.Config) func() {
	if cfg == nil {
		return func() {}
	}
	if serviceName == "" {
		serviceName = cfg.Service.Name
	}
	runtime, err := Init(serviceName, cfg.Observability)
	if err != nil {
		logger.Warn("可观测性初始化失败，服务继续启动", "service", serviceName, "error", err)
		return func() {}
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := runtime.Shutdown(ctx); err != nil {
			logger.Warn("可观测性关闭失败", "service", serviceName, "error", err)
		}
	}
}

func metricsHandler(registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func (r *Runtime) serve() {
	if r == nil || r.server == nil || r.listener == nil {
		return
	}
	err := r.server.Serve(r.listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Warn("可观测性指标端点停止", "service", r.ServiceName, "error", err)
	}
}

// Shutdown stops the metrics endpoint.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil || !r.Enabled || r.server == nil {
		return nil
	}
	var shutdownErr error
	if err := r.server.Shutdown(ctx); err != nil {
		shutdownErr = err
	}
	if r.tracerProvider != nil {
		if err := r.tracerProvider.Shutdown(ctx); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
	}
	if r.metricProvider != nil {
		if err := r.metricProvider.Shutdown(ctx); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
	}
	return shutdownErr
}

func setGlobal(runtime *Runtime) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalRuntime = runtime
}

func current() *Runtime {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalRuntime
}

// Shutdown stops the global runtime.
func Shutdown(ctx context.Context) error {
	runtime := current()
	if runtime == nil {
		return nil
	}
	return runtime.Shutdown(ctx)
}

// RecordDependencyCheck records startup dependency checks.
func RecordDependencyCheck(dependency, status string) {
	runtime := current()
	if runtime == nil || !runtime.Enabled {
		return
	}
	dependency = cleanLabel(dependency, "unknown")
	status = cleanLabel(status, "unknown")
	dependencyCheckTotal.WithLabelValues(runtime.ServiceName, dependency, status).Inc()
	if runtime.otelMetrics != nil {
		runtime.otelMetrics.dependencyCheck.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("service", runtime.ServiceName),
			attribute.String("dependency", dependency),
			attribute.String("status", status),
		))
	}
	updateUptime(runtime)
}

// RecordHTTPRequest records one HTTP request.
func RecordHTTPRequest(method, route string, status int, duration time.Duration) {
	runtime := current()
	if runtime == nil || !runtime.Enabled {
		return
	}
	statusText := strconv.Itoa(status)
	route = cleanRoute(route)
	method = cleanLabel(method, "UNKNOWN")
	httpRequestsTotal.WithLabelValues(runtime.ServiceName, method, route, statusText).Inc()
	httpDurationMS.WithLabelValues(runtime.ServiceName, method, route, statusText).Observe(float64(duration.Milliseconds()))
	if runtime.otelMetrics != nil {
		attrs := metric.WithAttributes(
			attribute.String("service", runtime.ServiceName),
			attribute.String("method", method),
			attribute.String("route", route),
			attribute.String("status", statusText),
		)
		runtime.otelMetrics.httpRequests.Add(context.Background(), 1, attrs)
		runtime.otelMetrics.httpDurationMS.Record(context.Background(), float64(duration.Milliseconds()), attrs)
	}
	updateUptime(runtime)
}

// RecordRPCRequest records one Kitex RPC call.
func RecordRPCRequest(service, method, status string, duration time.Duration) {
	runtime := current()
	if runtime == nil || !runtime.Enabled {
		return
	}
	service = cleanLabel(service, runtime.ServiceName)
	method = cleanLabel(method, "unknown")
	status = cleanLabel(status, "unknown")
	rpcRequestsTotal.WithLabelValues(service, method, status).Inc()
	rpcDurationMS.WithLabelValues(service, method, status).Observe(float64(duration.Milliseconds()))
	if runtime.otelMetrics != nil {
		attrs := metric.WithAttributes(
			attribute.String("service", service),
			attribute.String("method", method),
			attribute.String("status", status),
		)
		runtime.otelMetrics.rpcRequests.Add(context.Background(), 1, attrs)
		runtime.otelMetrics.rpcDurationMS.Record(context.Background(), float64(duration.Milliseconds()), attrs)
	}
	updateUptime(runtime)
}

// RecordBusinessEvent records a domain-level business event.
func RecordBusinessEvent(domain, operation, status string) {
	runtime := current()
	if runtime == nil || !runtime.Enabled {
		return
	}
	domain = cleanLabel(domain, "unknown")
	operation = cleanLabel(operation, "unknown")
	status = cleanLabel(status, "unknown")
	businessEventsTotal.WithLabelValues(runtime.ServiceName, domain, operation, status).Inc()
	if runtime.otelMetrics != nil {
		runtime.otelMetrics.businessEvents.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("service", runtime.ServiceName),
			attribute.String("domain", domain),
			attribute.String("operation", operation),
			attribute.String("status", status),
		))
	}
	updateUptime(runtime)
}

// RecordBusinessDuration records a domain-level business operation duration.
func RecordBusinessDuration(domain, operation, status string, duration time.Duration) {
	runtime := current()
	if runtime == nil || !runtime.Enabled {
		return
	}
	domain = cleanLabel(domain, "unknown")
	operation = cleanLabel(operation, "unknown")
	status = cleanLabel(status, "unknown")
	ms := float64(duration.Milliseconds())
	businessDurationMS.WithLabelValues(runtime.ServiceName, domain, operation, status).Observe(ms)
	if runtime.otelMetrics != nil {
		runtime.otelMetrics.businessDuration.Record(context.Background(), ms, metric.WithAttributes(
			attribute.String("service", runtime.ServiceName),
			attribute.String("domain", domain),
			attribute.String("operation", operation),
			attribute.String("status", status),
		))
	}
	updateUptime(runtime)
}

// SetBusinessGauge records a domain-level gauge value.
func SetBusinessGauge(domain, name string, value float64) {
	runtime := current()
	if runtime == nil || !runtime.Enabled {
		return
	}
	domain = cleanLabel(domain, "unknown")
	name = cleanLabel(name, "unknown")
	businessGauge.WithLabelValues(runtime.ServiceName, domain, name).Set(value)
	if runtime.otelMetrics != nil {
		runtime.otelMetrics.businessGauge.Record(context.Background(), value, metric.WithAttributes(
			attribute.String("service", runtime.ServiceName),
			attribute.String("domain", domain),
			attribute.String("name", name),
		))
	}
	updateUptime(runtime)
}

// StartSpan starts an OpenTelemetry span when tracing is enabled.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	runtime := current()
	if runtime == nil || !runtime.Enabled || runtime.tracer == nil {
		return noop.NewTracerProvider().Tracer("ClaranAIM/noop").Start(ctx, cleanLabel(name, "operation"), trace.WithAttributes(attrs...))
	}
	return runtime.tracer.Start(ctx, cleanLabel(name, "operation"), trace.WithAttributes(attrs...))
}

// Attribute creates a trace attribute without exposing OpenTelemetry imports to callers.
func Attribute(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
}

func updateUptime(runtime *Runtime) {
	if runtime == nil || runtime.startedAt.IsZero() {
		return
	}
	serviceUptimeSeconds.WithLabelValues(runtime.ServiceName).Set(time.Since(runtime.startedAt).Seconds())
}

func cleanLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func cleanRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return "unknown"
	}
	if strings.Contains(route, "?") {
		route = strings.Split(route, "?")[0]
	}
	return route
}
