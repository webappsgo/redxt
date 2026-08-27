package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WriteText renders every registered metric in Prometheus text
// exposition format (version 0.0.4), the format required by AI.md
// PART 21's prometheus service.
func (r *Registry) WriteText() string {
	return r.writeTextAt(time.Now())
}

// writeTextAt renders the registry as of now. It is split out from
// WriteText so tests can supply a fixed timestamp instead of
// time.Now(), per house style for time-based tests.
func (r *Registry) writeTextAt(now time.Time) string {
	r.snapshotUptime(now)
	r.snapshotDB()

	// Every read of a family's series/labelValues map happens under
	// r.mu, because Counter/Gauge/Histogram insert new series keys into
	// those same maps under the same lock. Snapshotting the pointers
	// here rather than ranging the maps later is what keeps a scrape
	// that overlaps a request carrying an unseen label combination from
	// tripping Go's concurrent map read/write fatal error.
	r.mu.Lock()
	counters := snapshotFamilies(r.counters)
	gauges := snapshotFamilies(r.gauges)
	hists := snapshotHistograms(r.histograms)
	r.mu.Unlock()

	var b strings.Builder
	for _, f := range counters {
		r.writeFamily(&b, f, "counter")
	}
	for _, f := range gauges {
		r.writeFamily(&b, f, "gauge")
	}
	for _, f := range hists {
		r.writeHistogram(&b, f)
	}
	return b.String()
}

// familySnapshot is one metric family captured under r.mu: the family
// name, its help text and its series in sorted key order. The values
// stay as pointers so the rendering pass still reads live numbers,
// which is safe because each value carries its own mutex.
type familySnapshot struct {
	name   string
	help   string
	labels []map[string]string
	values []*float64Value
}

// histogramSnapshot is the histogram equivalent of familySnapshot.
type histogramSnapshot struct {
	name    string
	help    string
	buckets []float64
	values  []*histogramValue
}

// snapshotFamilies captures every counter or gauge family. The caller
// must hold r.mu.
func snapshotFamilies(m map[string]*family) []familySnapshot {
	out := make([]familySnapshot, 0, len(m))
	for _, name := range sortedKeys(m) {
		f := m[name]
		snap := familySnapshot{name: name, help: f.help}
		keys := make([]string, 0, len(f.series))
		for k := range f.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			snap.labels = append(snap.labels, f.labelValues[k])
			snap.values = append(snap.values, f.series[k])
		}
		out = append(out, snap)
	}
	return out
}

// snapshotHistograms captures every histogram family. The caller must
// hold r.mu.
func snapshotHistograms(m map[string]*histogramFamily) []histogramSnapshot {
	out := make([]histogramSnapshot, 0, len(m))
	for _, name := range sortedHistKeys(m) {
		f := m[name]
		snap := histogramSnapshot{name: name, help: f.help, buckets: f.buckets}
		keys := make([]string, 0, len(f.series))
		for k := range f.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			snap.values = append(snap.values, f.series[k])
		}
		out = append(out, snap)
	}
	return out
}

// writeFamily renders one counter or gauge family. The sample name is
// the registered name verbatim: AI.md PART 21 requires counters to be
// registered as `..._total` already, so appending a suffix here would
// emit `..._total_total` and orphan the HELP/TYPE lines.
func (r *Registry) writeFamily(b *strings.Builder, f familySnapshot, typ string) {
	metric := r.metricName(f.name)
	fmt.Fprintf(b, "# HELP %s %s\n", metric, f.help)
	fmt.Fprintf(b, "# TYPE %s %s\n", metric, typ)

	for i, v := range f.values {
		fmt.Fprintf(b, "%s%s %s\n", metric, labelString(f.labels[i]), formatFloat(v.get()))
	}
}

func (r *Registry) writeHistogram(b *strings.Builder, f histogramSnapshot) {
	metric := r.metricName(f.name)
	fmt.Fprintf(b, "# HELP %s %s\n", metric, f.help)
	fmt.Fprintf(b, "# TYPE %s histogram\n", metric)

	for _, v := range f.values {
		v.mu.Lock()
		counts := append([]uint64(nil), v.counts...)
		sum := v.sum
		total := v.count
		lv := v.labelValues
		v.mu.Unlock()

		for i, bound := range f.buckets {
			labels := mergeLabels(lv, "le", formatFloat(bound))
			fmt.Fprintf(b, "%s_bucket%s %d\n", metric, labelString(labels), counts[i])
		}
		labels := mergeLabels(lv, "le", "+Inf")
		fmt.Fprintf(b, "%s_bucket%s %d\n", metric, labelString(labels), total)
		fmt.Fprintf(b, "%s_sum%s %s\n", metric, labelString(lv), formatFloat(sum))
		fmt.Fprintf(b, "%s_count%s %d\n", metric, labelString(lv), total)
	}
}

func mergeLabels(base map[string]string, key, value string) map[string]string {
	out := make(map[string]string, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out[key] = value
	return out
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func sortedKeys(m map[string]*family) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedHistKeys(m map[string]*histogramFamily) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
