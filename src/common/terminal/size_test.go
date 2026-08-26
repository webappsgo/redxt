package terminal

import "testing"

func TestCalculateMode(t *testing.T) {
	tests := []struct {
		name string
		cols int
		rows int
		want SizeMode
	}{
		{name: "zero dimensions", cols: 0, rows: 0, want: SizeMicro},
		{name: "cols 39 micro", cols: 39, rows: 100, want: SizeMicro},
		{name: "cols 40 minimal", cols: 40, rows: 100, want: SizeMinimal},
		{name: "rows 9 micro", cols: 1000, rows: 9, want: SizeMicro},
		{name: "rows 10 minimal", cols: 1000, rows: 10, want: SizeMinimal},
		{name: "cols 59 minimal", cols: 59, rows: 100, want: SizeMinimal},
		{name: "cols 60 compact", cols: 60, rows: 100, want: SizeCompact},
		{name: "rows 15 minimal", cols: 1000, rows: 15, want: SizeMinimal},
		{name: "rows 16 compact", cols: 1000, rows: 16, want: SizeCompact},
		{name: "cols 79 compact", cols: 79, rows: 100, want: SizeCompact},
		{name: "cols 80 standard", cols: 80, rows: 100, want: SizeStandard},
		{name: "rows 23 compact", cols: 1000, rows: 23, want: SizeCompact},
		{name: "rows 24 standard", cols: 1000, rows: 24, want: SizeStandard},
		{name: "cols 119 standard", cols: 119, rows: 100, want: SizeStandard},
		{name: "cols 120 wide", cols: 120, rows: 100, want: SizeWide},
		{name: "rows 39 standard", cols: 1000, rows: 39, want: SizeStandard},
		{name: "rows 40 wide", cols: 1000, rows: 40, want: SizeWide},
		{name: "cols 199 wide", cols: 199, rows: 100, want: SizeWide},
		{name: "cols 200 ultrawide", cols: 200, rows: 100, want: SizeUltrawide},
		{name: "rows 59 wide", cols: 1000, rows: 59, want: SizeWide},
		{name: "rows 60 ultrawide", cols: 1000, rows: 60, want: SizeUltrawide},
		{name: "cols 399 ultrawide", cols: 399, rows: 100, want: SizeUltrawide},
		{name: "cols 400 massive", cols: 400, rows: 100, want: SizeMassive},
		{name: "rows 79 ultrawide", cols: 1000, rows: 79, want: SizeUltrawide},
		{name: "rows 80 massive", cols: 1000, rows: 80, want: SizeMassive},
		{name: "default terminal is standard", cols: DefaultCols, rows: DefaultRows, want: SizeStandard},
		{name: "smaller dimension decides", cols: 500, rows: 12, want: SizeMinimal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateMode(tt.cols, tt.rows); got != tt.want {
				t.Errorf("calculateMode(%d, %d) = %v, want %v", tt.cols, tt.rows, got, tt.want)
			}
		})
	}
}

func TestSizeModeString(t *testing.T) {
	tests := []struct {
		name string
		mode SizeMode
		want string
	}{
		{name: "micro", mode: SizeMicro, want: "micro"},
		{name: "minimal", mode: SizeMinimal, want: "minimal"},
		{name: "compact", mode: SizeCompact, want: "compact"},
		{name: "standard", mode: SizeStandard, want: "standard"},
		{name: "wide", mode: SizeWide, want: "wide"},
		{name: "ultrawide", mode: SizeUltrawide, want: "ultrawide"},
		{name: "massive", mode: SizeMassive, want: "massive"},
		{name: "out of range", mode: SizeMode(99), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("SizeMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCapabilityHelpers(t *testing.T) {
	tests := []struct {
		name         string
		mode         SizeMode
		wantASCIIArt bool
		wantBorders  bool
		wantSidebar  bool
		wantIcons    bool
	}{
		{name: "micro", mode: SizeMicro},
		{name: "minimal", mode: SizeMinimal, wantIcons: true},
		{name: "compact", mode: SizeCompact, wantBorders: true, wantIcons: true},
		{name: "standard", mode: SizeStandard, wantASCIIArt: true, wantBorders: true, wantIcons: true},
		{
			name:         "wide",
			mode:         SizeWide,
			wantASCIIArt: true,
			wantBorders:  true,
			wantSidebar:  true,
			wantIcons:    true,
		},
		{
			name:         "ultrawide",
			mode:         SizeUltrawide,
			wantASCIIArt: true,
			wantBorders:  true,
			wantSidebar:  true,
			wantIcons:    true,
		},
		{
			name:         "massive",
			mode:         SizeMassive,
			wantASCIIArt: true,
			wantBorders:  true,
			wantSidebar:  true,
			wantIcons:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := TerminalSize{Cols: 80, Rows: 24, Mode: tt.mode}
			if got := size.ShowASCIIArt(); got != tt.wantASCIIArt {
				t.Errorf("ShowASCIIArt() = %v, want %v", got, tt.wantASCIIArt)
			}
			if got := size.ShowBorders(); got != tt.wantBorders {
				t.Errorf("ShowBorders() = %v, want %v", got, tt.wantBorders)
			}
			if got := size.ShowSidebar(); got != tt.wantSidebar {
				t.Errorf("ShowSidebar() = %v, want %v", got, tt.wantSidebar)
			}
			if got := size.ShowIcons(); got != tt.wantIcons {
				t.Errorf("ShowIcons() = %v, want %v", got, tt.wantIcons)
			}
			if got := tt.mode.ShowASCIIArt(); got != tt.wantASCIIArt {
				t.Errorf("SizeMode.ShowASCIIArt() = %v, want %v", got, tt.wantASCIIArt)
			}
		})
	}
}

func TestGetTerminalSizeFallsBackToEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		columns  string
		lines    string
		wantCols int
		wantRows int
	}{
		{name: "both set", columns: "132", lines: "50", wantCols: 132, wantRows: 50},
		{name: "unparsable values", columns: "abc", lines: "-4", wantCols: DefaultCols, wantRows: DefaultRows},
		{name: "unset values", columns: "", lines: "", wantCols: DefaultCols, wantRows: DefaultRows},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COLUMNS", tt.columns)
			t.Setenv("LINES", tt.lines)
			size := GetTerminalSize()
			if size.Cols <= 0 || size.Rows <= 0 {
				t.Fatalf("GetTerminalSize() = %+v, want positive dimensions", size)
			}
			if size.Mode != calculateMode(size.Cols, size.Rows) {
				t.Errorf("Mode = %v, want %v", size.Mode, calculateMode(size.Cols, size.Rows))
			}
			if _, _, ok := querySize(); ok {
				return
			}
			if size.Cols != tt.wantCols || size.Rows != tt.wantRows {
				t.Errorf("GetTerminalSize() = %dx%d, want %dx%d", size.Cols, size.Rows, tt.wantCols, tt.wantRows)
			}
		})
	}
}
