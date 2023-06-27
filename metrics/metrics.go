package metrics

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is a package-level singleton to track the state
// of all metrics generated by the process
type Metrics struct {
	namespace string
}

var mtx *Metrics

type MetricsMap[M prometheus.Metric, O any] struct {
	sync.RWMutex
	m           map[string]M
	initializor func(O) M
}

// MetricsRegistry proves a per-module api for creating
// and updating metrics
type MetricsRegistry struct {
	subsystem  string
	counters   MetricsMap[prometheus.Counter, prometheus.CounterOpts]
	gauges     MetricsMap[prometheus.Gauge, prometheus.GaugeOpts]
	histograms MetricsMap[prometheus.Histogram, prometheus.HistogramOpts]
}

// Init intializes the metrics package with the given namespace string.
// This should only be called once per process.
func Init(namespace string) (http.Handler, error) {
	if mtx != nil {
		return nil, fmt.Errorf("metrics.Init() should only be called once")
	}
	mtx = &Metrics{
		namespace,
	}

	// Initialize collection of node and validator count metrics
	InitEpochMetrics()

	return promhttp.Handler(), nil
}

func Deinit() {
	mtx = nil
}

// NewMetricsRegistry creates a new MetricsRegistry for a given module
// to use.
func NewMetricsRegistry(subsystem string) *MetricsRegistry {
	return &MetricsRegistry{
		subsystem: subsystem,
		counters: MetricsMap[prometheus.Counter, prometheus.CounterOpts]{
			m:           make(map[string]prometheus.Counter),
			initializor: promauto.NewCounter,
		},
		gauges: MetricsMap[prometheus.Gauge, prometheus.GaugeOpts]{
			m:           make(map[string]prometheus.Gauge),
			initializor: promauto.NewGauge,
		},
		histograms: MetricsMap[prometheus.Histogram, prometheus.HistogramOpts]{
			m:           make(map[string]prometheus.Histogram),
			initializor: promauto.NewHistogram,
		},
	}
}

func (m *MetricsMap[T, O]) value(name string, opts O) T {

	m.RLock()
	val, ok := m.m[name]
	m.RUnlock()

	if ok {
		return val
	}

	// Escalate to a full lock
	m.Lock()
	defer m.Unlock()

	val, ok = m.m[name]
	if ok {
		// Someone else created the metric while we were
		// upgrading our lock.
		return val
	}

	val = m.initializor(opts)
	m.m[name] = val
	return val
}

// Counter creates or fetches a prometheus Counter from the metrics
// registry and returns it.
func (m *MetricsRegistry) Counter(name string) prometheus.Counter {

	return m.counters.value(name, prometheus.CounterOpts{
		Namespace: mtx.namespace,
		Subsystem: m.subsystem,
		Name:      name,
	})
}

// Gauge creates or fetches a prometheus Gauge from the metrics
// registry and returns it.
func (m *MetricsRegistry) Gauge(name string) prometheus.Gauge {

	return m.gauges.value(name, prometheus.GaugeOpts{
		Namespace: mtx.namespace,
		Subsystem: m.subsystem,
		Name:      name,
	})
}

func (m *MetricsRegistry) GaugeFunc(name string, handler func() float64) {
	_ = promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: mtx.namespace,
		Subsystem: m.subsystem,
		Name:      name,
	}, handler)
}

// Histogram creates or fetches a prometheus Histogram from the metrics
// registry and returns it.
func (m *MetricsRegistry) Histogram(name string) prometheus.Histogram {

	return m.histograms.value(name, prometheus.HistogramOpts{
		Namespace: mtx.namespace,
		Subsystem: m.subsystem,
		Name:      name,
	})
}
