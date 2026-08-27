package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestWriteTextAtRendersCounterGaugeHistogram(t *testing.T) {
	r := New("redxt")
	r.Counter("requests_total", "Total requests.", map[string]string{"status": "200"}, 3)
	r.Gauge("active_requests", "In flight.", nil, 2)
	r.Histogram("duration_seconds", "Latency.", []float64{0.5, 1}, nil, 0.2)

	fixed := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	out := r.writeTextAt(fixed)

	for _, want := range []string{
		"# HELP redxt_requests_total Total requests.",
		"# TYPE redxt_requests_total counter",
		`redxt_requests_total_total{status="200"} 3`,
		"# TYPE redxt_active_requests gauge",
		"redxt_active_requests 2",
		"# TYPE redxt_duration_seconds histogram",
		`redxt_duration_seconds_bucket{le="0.5"} 1`,
		`redxt_duration_seconds_bucket{le="1"} 1`,
		`redxt_duration_seconds_bucket{le="+Inf"} 1`,
		"redxt_duration_seconds_sum 0.2",
		"redxt_duration_seconds_count 1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n--- full output ---\n%s", want, out)
		}
	}
}

func TestWriteTextAtIncludesUptimeAfterRegisterApp(t *testing.T) {
	r := New("redxt")
	start := time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC)
	r.RegisterApp(AppInfo{Version: "1.0.0"}, start)

	now := start.Add(90 * time.Second)
	out := r.writeTextAt(now)

	if !strings.Contains(out, "redxt_app_uptime_seconds 90") {
		t.Fatalf("expected uptime of 90 seconds in output:\n%s", out)
	}
	if !strings.Contains(out, `version="1.0.0"`) {
		t.Fatalf("expected version label in output:\n%s", out)
	}
}

func TestFormatFloatMinimalForm(t *testing.T) {
	if got := formatFloat(3); got != "3" {
		t.Fatalf("formatFloat(3) = %q, want 3", got)
	}
	if got := formatFloat(0.2); got != "0.2" {
		t.Fatalf("formatFloat(0.2) = %q, want 0.2", got)
	}
}

func TestMergeLabelsDoesNotMutateBase(t *testing.T) {
	base := map[string]string{"a": "1"}
	merged := mergeLabels(base, "le", "0.5")
	if len(base) != 1 {
		t.Fatalf("base map was mutated: %v", base)
	}
	if merged["le"] != "0.5" || merged["a"] != "1" {
		t.Fatalf("merged = %v", merged)
	}
}
