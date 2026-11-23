// Package metrics exposes Prometheus-compatible metrics for HTTP services.
//
// It is backed by github.com/VictoriaMetrics/metrics, which fits this
// library's small-dependency style: a single module with two tiny transitive
// dependencies and no protobuf/procfs tree, while still emitting standard
// Prometheus text exposition that VictoriaMetrics (vmagent) and Prometheus
// scrape unchanged.
package metrics

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	vmmetrics "github.com/VictoriaMetrics/metrics"
)

var (
	validMetricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	validLabelName  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// Labels are metric label name/value pairs. Keep values bounded: never use
// raw paths, IDs, or other unbounded input as a label value.
type Labels map[string]string

// Registry is an isolated set of metrics. Create one per service with New.
type Registry struct {
	set     *vmmetrics.Set
	gauges  map[string]*Gauge
	counter map[string]*Counter
	hist    map[string]*Histogram
	mu      sync.Mutex
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{
		set:     vmmetrics.NewSet(),
		gauges:  make(map[string]*Gauge),
		counter: make(map[string]*Counter),
		hist:    make(map[string]*Histogram),
	}
}

// Handler serves the registry in Prometheus text format at any path it is
// mounted on, including Go runtime and process metrics.
//
//	mux.Handle("/metrics", registry.Handler())
func (registry *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		registry.set.WritePrometheus(writer)
		vmmetrics.WriteProcessMetrics(writer)
	})
}

// WritePrometheus writes only the registry's own metrics to writer,
// without Go runtime and process metrics.
func (registry *Registry) WritePrometheus(writer io.Writer) {
	registry.set.WritePrometheus(writer)
}

// Counter returns the counter for name and labels, creating it if needed.
// It panics on an invalid metric or label name.
func (registry *Registry) Counter(name string, labels Labels) *Counter {
	fullName := metricName(name, labels)

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if counter, ok := registry.counter[fullName]; ok {
		return counter
	}

	counter := &Counter{inner: registry.set.GetOrCreateCounter(fullName)}
	registry.counter[fullName] = counter

	return counter
}

// Gauge returns the gauge for name and labels, creating it if needed.
// It panics on an invalid metric or label name.
func (registry *Registry) Gauge(name string, labels Labels) *Gauge {
	fullName := metricName(name, labels)

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if gauge, ok := registry.gauges[fullName]; ok {
		return gauge
	}

	gauge := &Gauge{}
	registry.set.GetOrCreateGauge(fullName, gauge.Get)
	registry.gauges[fullName] = gauge

	return gauge
}

// Histogram returns the Prometheus-style histogram for name and labels,
// creating it if needed. It panics on an invalid metric or label name.
func (registry *Registry) Histogram(name string, labels Labels) *Histogram {
	fullName := metricName(name, labels)

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if histogram, ok := registry.hist[fullName]; ok {
		return histogram
	}

	histogram := &Histogram{
		inner: registry.set.GetOrCreatePrometheusHistogram(fullName),
	}
	registry.hist[fullName] = histogram

	return histogram
}

// Counter is a monotonically increasing counter.
type Counter struct {
	inner *vmmetrics.Counter
}

// Inc increments the counter by one.
func (counter *Counter) Inc() {
	counter.inner.Inc()
}

// Add increments the counter by delta. It panics if delta is negative.
func (counter *Counter) Add(delta int) {
	counter.inner.Add(delta)
}

// Get returns the current counter value.
func (counter *Counter) Get() uint64 {
	return counter.inner.Get()
}

// Gauge is a value that can go up and down.
type Gauge struct {
	value atomic.Uint64
}

// Set sets the gauge to value.
func (gauge *Gauge) Set(value float64) {
	gauge.value.Store(math.Float64bits(value))
}

// Add adds delta to the gauge.
func (gauge *Gauge) Add(delta float64) {
	for {
		old := gauge.value.Load()
		if gauge.value.CompareAndSwap(old, math.Float64bits(math.Float64frombits(old)+delta)) {
			return
		}
	}
}

// Inc increments the gauge by one.
func (gauge *Gauge) Inc() {
	gauge.Add(1)
}

// Dec decrements the gauge by one.
func (gauge *Gauge) Dec() {
	gauge.Add(-1)
}

// Get returns the current gauge value.
func (gauge *Gauge) Get() float64 {
	return math.Float64frombits(gauge.value.Load())
}

// Histogram observes values in Prometheus bucket format.
type Histogram struct {
	inner *vmmetrics.PrometheusHistogram
}

// Observe records one observation.
func (histogram *Histogram) Observe(value float64) {
	histogram.inner.Update(value)
}

func metricName(name string, labels Labels) string {
	if !validMetricName.MatchString(name) {
		panic(fmt.Sprintf("metrics: invalid metric name %q", name))
	}

	if len(labels) == 0 {
		return name
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		if !validLabelName.MatchString(key) {
			panic(fmt.Sprintf("metrics: invalid label name %q", key))
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString(name)
	builder.WriteByte('{')

	for index, key := range keys {
		if index > 0 {
			builder.WriteByte(',')
		}

		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(strconv.Quote(labels[key]))
	}

	builder.WriteByte('}')

	return builder.String()
}
