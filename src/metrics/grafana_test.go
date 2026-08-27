package metrics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDashboardBuildsValidJSONWithPrefixedExpressions(t *testing.T) {
	body, err := Dashboard("redxt", "redxt metrics")
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["title"] != "redxt metrics" {
		t.Fatalf("title = %v, want %q", doc["title"], "redxt metrics")
	}
	panels, ok := doc["panels"].([]any)
	if !ok || len(panels) == 0 {
		t.Fatalf("expected non-empty panels array, got %v", doc["panels"])
	}
	if !strings.Contains(string(body), "redxt_http_requests_total") {
		t.Fatalf("expected a redxt_-prefixed expression in dashboard JSON")
	}
}
