package service

import (
	"runtime"
	"testing"
)

func TestNewBuildsProductionManager(t *testing.T) {
	log := noopLogger{}
	m := New(Context{InternalName: "redxt"}, true, log)

	if m.GOOS != runtime.GOOS {
		t.Errorf("GOOS = %q, want %q", m.GOOS, runtime.GOOS)
	}
	if m.Root != "" {
		t.Errorf("Root = %q, want empty (real filesystem)", m.Root)
	}
	if !m.Elevated {
		t.Error("Elevated = false, want true")
	}
	if m.Runner == nil || m.IDLU == nil || m.PathLU == nil || m.FileLU == nil {
		t.Fatal("New must populate every OS-facing dependency")
	}
	if m.Confirm == nil {
		t.Fatal("New must populate Confirm")
	}
	if m.Confirm("anything") {
		t.Error("defaultConfirm must always decline")
	}
}

func TestLogfNoopWhenLogNil(t *testing.T) {
	m := &Manager{}
	// Must not panic when Log is nil.
	m.logf("info", "hello %s", "world")
	m.logf("warn", "hello %s", "world")
	m.logf("error", "hello %s", "world")
}

// recordingLogger records the level and message of every call, so
// TestLogfDispatchesToCorrectLevel can assert logf routes to the right
// Logger method.
type recordingLogger struct {
	infos, warns, errors []string
}

func (r *recordingLogger) Infof(format string, args ...any)  { r.infos = append(r.infos, format) }
func (r *recordingLogger) Warnf(format string, args ...any)  { r.warns = append(r.warns, format) }
func (r *recordingLogger) Errorf(format string, args ...any) { r.errors = append(r.errors, format) }

func TestLogfDispatchesToCorrectLevel(t *testing.T) {
	rec := &recordingLogger{}
	m := &Manager{Log: rec}

	m.logf("warn", "w")
	m.logf("error", "e")
	m.logf("info", "i")
	m.logf("bogus-level-defaults-to-info", "d")

	if len(rec.warns) != 1 || rec.warns[0] != "w" {
		t.Errorf("warns = %v, want [w]", rec.warns)
	}
	if len(rec.errors) != 1 || rec.errors[0] != "e" {
		t.Errorf("errors = %v, want [e]", rec.errors)
	}
	if len(rec.infos) != 2 || rec.infos[0] != "i" || rec.infos[1] != "d" {
		t.Errorf("infos = %v, want [i d]", rec.infos)
	}
}

func TestManagerDetectInit(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	init, err := m.detectInit()
	if err != nil {
		t.Fatalf("detectInit: %v", err)
	}
	if init != InitSystemd {
		t.Errorf("detectInit = %q, want %q", init, InitSystemd)
	}
}
