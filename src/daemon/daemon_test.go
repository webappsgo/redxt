package daemon

import (
	"reflect"
	"testing"
)

// TestFilterDaemonFlag verifies every recognized spelling of the daemon
// flag is removed while unrelated arguments, including ones that merely
// contain the substring "daemon", are preserved in order.
func TestFilterDaemonFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "empty input",
			args: []string{},
			want: []string{},
		},
		{
			name: "long form",
			args: []string{"--daemon"},
			want: []string{},
		},
		{
			name: "short form",
			args: []string{"-daemon"},
			want: []string{},
		},
		{
			name: "long form equals true",
			args: []string{"--daemon=true"},
			want: []string{},
		},
		{
			name: "long form equals false",
			args: []string{"--daemon=false"},
			want: []string{},
		},
		{
			name: "short form equals true",
			args: []string{"-daemon=true"},
			want: []string{},
		},
		{
			name: "short form equals false",
			args: []string{"-daemon=false"},
			want: []string{},
		},
		{
			name: "adjacent unrelated flags preserved in order",
			args: []string{"--verbose", "--daemon", "--config", "/etc/redxt.yaml"},
			want: []string{"--verbose", "--config", "/etc/redxt.yaml"},
		},
		{
			name: "value taking flag left intact",
			args: []string{"--port", "8053", "--daemon"},
			want: []string{"--port", "8053"},
		},
		{
			name: "argument literally named --daemonize is not removed",
			args: []string{"--daemonize"},
			want: []string{"--daemonize"},
		},
		{
			name: "argument literally named --daemonize=true is not removed",
			args: []string{"--daemonize=true"},
			want: []string{"--daemonize=true"},
		},
		{
			name: "no daemon flag present",
			args: []string{"--status", "--verbose"},
			want: []string{"--status", "--verbose"},
		},
		{
			name: "multiple daemon flags removed",
			args: []string{"--daemon", "--verbose", "-daemon"},
			want: []string{"--verbose"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterDaemonFlag(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FilterDaemonFlag(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// TestFilterDaemonFlagDoesNotMutateInput ensures the returned slice does
// not share backing storage that could corrupt the caller's argument list.
func TestFilterDaemonFlagDoesNotMutateInput(t *testing.T) {
	original := []string{"--daemon", "--verbose"}
	snapshot := append([]string{}, original...)

	_ = FilterDaemonFlag(original)

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("FilterDaemonFlag mutated its input: got %v, want %v", original, snapshot)
	}
}

// TestIsChild verifies IsChild reflects the daemon child environment
// marker in both the set and unset states.
func TestIsChild(t *testing.T) {
	t.Run("marker unset", func(t *testing.T) {
		t.Setenv(daemonChildEnvVar, "")
		if IsChild() {
			t.Errorf("IsChild() = true, want false when marker is unset")
		}
	})

	t.Run("marker set", func(t *testing.T) {
		t.Setenv(daemonChildEnvVar, "1")
		if !IsChild() {
			t.Errorf("IsChild() = false, want true when marker is set")
		}
	})
}

// TestDaemonizeRefusesWhenAlreadyChild verifies Daemonize returns an error
// instead of re-execing again when the current process is already the
// daemon child. No real process is forked by this test.
func TestDaemonizeRefusesWhenAlreadyChild(t *testing.T) {
	t.Setenv(daemonChildEnvVar, "1")

	pid, err := Daemonize(Options{})
	if err == nil {
		t.Fatal("Daemonize() returned nil error when already the daemon child")
	}
	if pid != 0 {
		t.Errorf("Daemonize() pid = %d, want 0 on error", pid)
	}
}
