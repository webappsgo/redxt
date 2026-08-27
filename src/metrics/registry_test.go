package metrics

import "testing"

func TestCounterAccumulates(t *testing.T) {
	r := New("redxt")
	r.Counter("requests_total", "help", map[string]string{"a": "1"}, 2)
	r.Counter("requests_total", "help", map[string]string{"a": "1"}, 3)
	r.Counter("requests_total", "help", map[string]string{"a": "2"}, 5)

	r.mu.Lock()
	f := r.counters["requests_total"]
	r.mu.Unlock()
	if got := f.series[seriesKey(map[string]string{"a": "1"})].get(); got != 5 {
		t.Fatalf("series a=1 = %v, want 5", got)
	}
	if got := f.series[seriesKey(map[string]string{"a": "2"})].get(); got != 5 {
		t.Fatalf("series a=2 = %v, want 5", got)
	}
}

func TestCounterRejectsNegativeDelta(t *testing.T) {
	r := New("redxt")
	r.Counter("c", "help", nil, -5)
	r.mu.Lock()
	f := r.counters["c"]
	r.mu.Unlock()
	if got := f.series[seriesKey(nil)].get(); got != 0 {
		t.Fatalf("negative delta should be clamped to 0, got %v", got)
	}
}

func TestGaugeOverwrites(t *testing.T) {
	r := New("redxt")
	r.Gauge("g", "help", nil, 1)
	r.Gauge("g", "help", nil, 42)
	r.mu.Lock()
	f := r.gauges["g"]
	r.mu.Unlock()
	if got := f.series[seriesKey(nil)].get(); got != 42 {
		t.Fatalf("gauge = %v, want 42", got)
	}
}

func TestHistogramBucketsAndSum(t *testing.T) {
	r := New("redxt")
	buckets := []float64{0.1, 0.5, 1}
	r.Histogram("h", "help", buckets, nil, 0.05)
	r.Histogram("h", "help", buckets, nil, 0.4)
	r.Histogram("h", "help", buckets, nil, 2)

	r.mu.Lock()
	f := r.histograms["h"]
	r.mu.Unlock()
	v := f.series[seriesKey(nil)]
	if v.count != 3 {
		t.Fatalf("count = %d, want 3", v.count)
	}
	if v.sum != 0.05+0.4+2 {
		t.Fatalf("sum = %v, want %v", v.sum, 0.05+0.4+2)
	}
	// 0.05 falls in bucket 0 (<=0.1) and bucket 1 (<=0.5); 0.4 falls in
	// bucket 1 only; 2 falls in neither bucket (only +Inf).
	if v.counts[0] != 1 {
		t.Fatalf("bucket 0.1 count = %d, want 1", v.counts[0])
	}
	if v.counts[1] != 2 {
		t.Fatalf("bucket 0.5 count = %d, want 2", v.counts[1])
	}
	if v.counts[2] != 2 {
		t.Fatalf("bucket 1 count = %d, want 2", v.counts[2])
	}
}

func TestSeriesKeyStableRegardlessOfInsertOrder(t *testing.T) {
	a := seriesKey(map[string]string{"x": "1", "y": "2"})
	b := seriesKey(map[string]string{"y": "2", "x": "1"})
	if a != b {
		t.Fatalf("seriesKey not stable: %q != %q", a, b)
	}
}

func TestMetricNamePrefixed(t *testing.T) {
	r := New("redxt")
	if got := r.metricName("app_info"); got != "redxt_app_info" {
		t.Fatalf("metricName = %q, want redxt_app_info", got)
	}
}
