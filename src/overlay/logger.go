package overlay

// Logger is the subset of *logging.Logger this package needs. Taking a
// narrow local interface, rather than importing src/logging directly, keeps
// this package free to evolve independently (same pattern as
// src/scheduler.Logger).
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// discardLogger is used when a caller passes a nil Logger, so every method on
// TorManager/I2PManager stays nil-safe without scattering nil checks.
type discardLogger struct{}

func (discardLogger) Debugf(string, ...any) {}
func (discardLogger) Infof(string, ...any)  {}
func (discardLogger) Warnf(string, ...any)  {}
func (discardLogger) Errorf(string, ...any) {}

// orDiscard returns log unless it is nil, in which case it returns a Logger
// whose methods are no-ops.
func orDiscard(log Logger) Logger {
	if log == nil {
		return discardLogger{}
	}
	return log
}
