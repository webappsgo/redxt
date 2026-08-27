package metrics

import "encoding/json"

// grafanaPanel is a minimal Grafana panel definition: one PromQL
// query per panel, enough for an operator to import and immediately
// see data without hand-editing the dashboard.
type grafanaPanel struct {
	ID    int                  `json:"id"`
	Title string               `json:"title"`
	Type  string               `json:"type"`
	GridX int                  `json:"gridPos_x"`
	GridY int                  `json:"gridPos_y"`
	Targs []grafanaPanelTarget `json:"targets"`
}

type grafanaPanelTarget struct {
	Expr string `json:"expr"`
	Refs string `json:"refId"`
}

// grafanaCategory groups the panels for one PART 21 metrics category.
type grafanaCategory struct {
	title string
	panel string // "graph" or "stat"
	exprs []string
}

// Dashboard builds a complete, importable Grafana dashboard JSON
// document covering every PART 21 metrics category (HTTP, database,
// cache, scheduler, system, business), using prefix as the metric
// name prefix. The datasource is left as the standard Grafana
// template variable ${DS_PROMETHEUS} so the dashboard imports against
// any Prometheus datasource.
func Dashboard(prefix, title string) ([]byte, error) {
	categories := []grafanaCategory{
		{"HTTP", "graph", []string{
			"sum(rate(" + prefix + "_http_requests_total[5m])) by (status)",
			"histogram_quantile(0.95, sum(rate(" + prefix + "_http_request_duration_seconds_bucket[5m])) by (le))",
			prefix + "_http_active_requests",
		}},
		{"Database", "graph", []string{
			"sum(rate(" + prefix + "_db_queries_total[5m])) by (operation)",
			prefix + "_db_connections_open",
			prefix + "_db_connections_in_use",
			"sum(rate(" + prefix + "_db_errors_total[5m])) by (error_type)",
		}},
		{"Cache", "graph", []string{
			"sum(rate(" + prefix + "_cache_hits_total[5m]))",
			"sum(rate(" + prefix + "_cache_misses_total[5m]))",
		}},
		{"Scheduler", "graph", []string{
			"sum(rate(" + prefix + "_scheduler_task_runs_total[5m])) by (task, status)",
			"sum(rate(" + prefix + "_scheduler_task_failures_total[5m])) by (task)",
		}},
		{"System", "graph", []string{
			prefix + "_system_cpu_usage_ratio",
			prefix + "_system_memory_usage_bytes",
			"go_goroutines",
		}},
		{"Business", "stat", []string{
			prefix + "_auth_sessions_active",
			"sum(rate(" + prefix + "_auth_attempts_total[5m])) by (status)",
		}},
	}

	var panels []grafanaPanel
	id := 1
	y := 0
	for _, cat := range categories {
		var targets []grafanaPanelTarget
		for i, expr := range cat.exprs {
			targets = append(targets, grafanaPanelTarget{Expr: expr, Refs: string(rune('A' + i))})
		}
		panels = append(panels, grafanaPanel{
			ID:    id,
			Title: cat.title,
			Type:  cat.panel,
			GridX: 0,
			GridY: y,
			Targs: targets,
		})
		id++
		y += 8
	}

	doc := map[string]any{
		"title":         title,
		"schemaVersion": 39,
		"version":       1,
		"editable":      true,
		"panels":        panels,
		"templating": map[string]any{
			"list": []map[string]any{
				{
					"name":  "DS_PROMETHEUS",
					"type":  "datasource",
					"query": "prometheus",
				},
			},
		},
	}
	return json.Marshal(doc)
}
