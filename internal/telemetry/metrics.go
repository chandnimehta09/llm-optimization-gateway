package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all custom Prometheus metrics for the gateway.
type Metrics struct {
	RequestsTotal       prometheus.Counter
	RequestDuration     prometheus.Histogram
	ActiveRequests      prometheus.Gauge
	ProviderCallsTotal  *prometheus.CounterVec
	ProviderErrorsTotal *prometheus.CounterVec
	CacheHitsTotal      prometheus.Counter
	CacheMissesTotal    prometheus.Counter

	registry *prometheus.Registry
}

// InitMetrics creates a new Prometheus registry, registers all gateway metrics,
// and returns the Metrics instance along with an HTTP handler for scraping.
func InitMetrics(cfg Config) (*Metrics, http.Handler, error) {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		RequestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total number of requests received by the gateway.",
		}),
		RequestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Histogram of request durations in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		ActiveRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_active_requests",
			Help: "Number of currently active requests.",
		}),
		ProviderCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_provider_calls_total",
			Help: "Total number of calls made to upstream providers.",
		}, []string{"provider"}),
		ProviderErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_provider_errors_total",
			Help: "Total number of errors from upstream providers.",
		}, []string{"provider", "error_type"}),
		CacheHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_cache_hits_total",
			Help: "Total number of cache hits.",
		}),
		CacheMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_cache_misses_total",
			Help: "Total number of cache misses.",
		}),
		registry: reg,
	}

	collectors := []prometheus.Collector{
		m.RequestsTotal,
		m.RequestDuration,
		m.ActiveRequests,
		m.ProviderCallsTotal,
		m.ProviderErrorsTotal,
		m.CacheHitsTotal,
		m.CacheMissesTotal,
	}

	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			return nil, nil, err
		}
	}

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: false,
	})

	return m, handler, nil
}

// Registry returns the underlying Prometheus registry.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
