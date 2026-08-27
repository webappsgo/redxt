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

	r.mu.Lock()
	counterNames := sortedKeys(r.counters)
	gaugeNames := sortedKeys(r.gauges)
	histNames := sortedHistKeys(r.histograms)
	r.mu.Unlock()

	var b strings.Builder
	for _, name := range counterNames {
		r.mu.Lock()
		f := r.counters[name]
		r.mu.Unlock()
		r.writeFamily(&b, name, "counter", f, true)
	}
	for _, name := range gaugeNames {
		r.mu.Lock()
		f := r.gauges[name]
		r.mu.Unlock()
		r.writeFamily(&b, name, "gauge", f, false)
	}
	for _, name := range histNames {
		r.mu.Lock()
		f := r.histograms[name]
		r.mu.Unlock()
		r.writeHistogram(&b, name, f)
	}
	return b.String()
}

func (r *Registry) writeFamily(b *strings.Builder, name, typ string, f *family, isCounter bool) {
	metric := r.metricName(name)
	fmt.Fprintf(b, "# HELP %s %s\n", metric, f.help)
	fmt.Fprintf(b, "# TYPE %s %s\n", metric, typ)

	keys := make([]string, 0, len(f.series))
	for k := range f.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		suffix := ""
		if isCounter {
			suffix = "_total"
		}
		lv := f.labelValues[k]
		fmt.Fprintf(b, "%s%s%s %s\n", metric, suffix, labelString(lv), formatFloat(f.series[k].get()))
	}
}

func (r *Registry) writeHistogram(b *strings.Builder, name string, f *histogramFamily) {
	metric := r.metricName(name)
	fmt.Fprintf(b, "# HELP %s %s\n", metric, f.help)
	fmt.Fprintf(b, "# TYPE %s histogram\n", metric)

	keys := make([]string, 0, len(f.series))
	for k := range f.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := f.series[k]
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
