package service

import "runtime"

// New builds a production Manager: real command execution, real os/user
// and os/exec lookups, the current process's GOOS, and no root prefix
// (writes go to the real filesystem). Tests build a Manager literal
// directly instead, injecting fakes and a t.TempDir() Root.
func New(ctx Context, elevated bool, log Logger) *Manager {
	return &Manager{
		Ctx:      ctx,
		Runner:   NewExecRunner(),
		Log:      log,
		GOOS:     runtime.GOOS,
		Root:     "",
		IDLU:     NewOSIDLookup(),
		PathLU:   NewOSPathLookup(),
		FileLU:   NewOSFileLookup(),
		Elevated: elevated,
		Confirm:  defaultConfirm,
	}
}

// defaultConfirm always declines - production callers should pass their
// own confirmation function (reading from a terminal) rather than rely on
// this fail-closed default.
func defaultConfirm(string) bool { return false }

// detectInit resolves the init system to target for m.GOOS.
func (m *Manager) detectInit() (InitSystem, error) {
	return DetectInit(m.GOOS, m.FileLU, m.PathLU)
}

// logf logs via m.Log when set, and is a no-op otherwise.
func (m *Manager) logf(level string, format string, args ...any) {
	if m.Log == nil {
		return
	}
	switch level {
	case "warn":
		m.Log.Warnf(format, args...)
	case "error":
		m.Log.Errorf(format, args...)
	default:
		m.Log.Infof(format, args...)
	}
}
