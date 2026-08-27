// Package metrics implements AI.md PART 21: a Prometheus-compatible
// /server/metrics surface plus its grafana and loki sibling services,
// all served from a single in-process registry.
//
// The registry is a small hand-rolled implementation rather than
// github.com/prometheus/client_golang: it needs only counters, gauges,
// and histograms in the Prometheus text exposition format, and
// avoiding the dependency keeps the module's supply chain smaller
// without giving up compatibility — the wire format is what
// Prometheus and Grafana actually parse.
package metrics

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry holds every metric family this process exports. It is safe
// for concurrent use: HTTP handlers, the scheduler, and background
// collectors all write to the same Registry from different
// goroutines.
type Registry struct {
	prefix string

	mu         sync.Mutex
	counters   map[string]*family
	gauges     map[string]*family
	histograms map[string]*histogramFamily

	uptimeStart time.Time
	dbSources   map[string]DBStatsSource

	activeMu sync.Mutex
	active   float64
}

// New returns an empty Registry whose metric names are all prefixed
// with prefix+"_", per AI.md PART 21's naming convention.
func New(prefix string) *Registry {
	return &Registry{
		prefix:     prefix,
		counters:   map[string]*family{},
		gauges:     map[string]*family{},
		histograms: map[string]*histogramFamily{},
	}
}

// family holds every label-combination series for one metric name.
type family struct {
	help        string
	series      map[string]*float64Value
	labelValues map[string]map[string]string
}

type float64Value struct {
	mu  sync.Mutex
	val float64
}

func (v *float64Value) add(delta float64) {
	v.mu.Lock()
	v.val += delta
	v.mu.Unlock()
}

func (v *float64Value) set(val float64) {
	v.mu.Lock()
	v.val = val
	v.mu.Unlock()
}

func (v *float64Value) get() float64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.val
}

// seriesKey renders a stable, ordered key for a label value set so
// the same combination always maps to the same series regardless of
// the order the caller passed labelValues in.
func seriesKey(labelValues map[string]string) string {
	keys := make([]string, 0, len(labelValues))
	for k := range labelValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labelValues[k])
		b.WriteByte(';')
	}
	return b.String()
}

func labelString(labelValues map[string]string) string {
	if len(labelValues) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labelValues))
	for k := range labelValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+`="`+escapeLabelValue(labelValues[k])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// labelValueEscaper implements the only three escapes the Prometheus
// text exposition parser accepts. Go's %q verb is not a substitute: it
// also escapes non-ASCII runes as \xNN/\uNNNN, which the parser does
// not recognize, silently corrupting UTF-8 label values such as an ASN
// organization name.
var labelValueEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)

// escapeLabelValue renders v safe to place inside a quoted label value.
func escapeLabelValue(v string) string {
	return labelValueEscaper.Replace(v)
}

// Counter registers name (without the registry prefix) as a counter
// family if it does not already exist, then adds delta to the series
// identified by labelValues. delta must be >= 0.
func (r *Registry) Counter(name, help string, labelValues map[string]string, delta float64) {
	if delta < 0 {
		delta = 0
	}
	r.mu.Lock()
	f, ok := r.counters[name]
	if !ok {
		f = &family{help: help, series: map[string]*float64Value{}, labelValues: map[string]map[string]string{}}
		r.counters[name] = f
	}
	key := seriesKey(labelValues)
	v, ok := f.series[key]
	if !ok {
		v = &float64Value{}
		f.series[key] = v
		f.labelValues[key] = labelValues
	}
	r.mu.Unlock()
	v.add(delta)
}

// Gauge registers name as a gauge family if needed, then sets the
// series identified by labelValues to value.
func (r *Registry) Gauge(name, help string, labelValues map[string]string, value float64) {
	r.mu.Lock()
	f, ok := r.gauges[name]
	if !ok {
		f = &family{help: help, series: map[string]*float64Value{}, labelValues: map[string]map[string]string{}}
		r.gauges[name] = f
	}
	key := seriesKey(labelValues)
	v, ok := f.series[key]
	if !ok {
		v = &float64Value{}
		f.series[key] = v
		f.labelValues[key] = labelValues
	}
	r.mu.Unlock()
	v.set(value)
}

type histogramFamily struct {
	help    string
	buckets []float64
	series  map[string]*histogramValue
}

type histogramValue struct {
	mu          sync.Mutex
	labelValues map[string]string
	counts      []uint64 // per-bucket cumulative-eligible counts, same length as buckets
	sum         float64
	count       uint64
}

// Histogram registers name as a histogram family with the given
// bucket boundaries (seconds or bytes, per the caller) if needed, then
// records one observation.
func (r *Registry) Histogram(name, help string, buckets []float64, labelValues map[string]string, observation float64) {
	r.mu.Lock()
	f, ok := r.histograms[name]
	if !ok {
		f = &histogramFamily{help: help, buckets: append([]float64(nil), buckets...), series: map[string]*histogramValue{}}
		r.histograms[name] = f
	}
	key := seriesKey(labelValues)
	v, ok := f.series[key]
	if !ok {
		v = &histogramValue{labelValues: labelValues, counts: make([]uint64, len(f.buckets))}
		f.series[key] = v
	}
	r.mu.Unlock()

	v.mu.Lock()
	for i, bound := range f.buckets {
		if observation <= bound {
			v.counts[i]++
		}
	}
	v.sum += observation
	v.count++
	v.mu.Unlock()
}

// metricName joins the registry prefix and the metric name.
func (r *Registry) metricName(name string) string {
	return r.prefix + "_" + name
}
