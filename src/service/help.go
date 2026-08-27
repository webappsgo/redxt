package service

import "fmt"

// serviceHelpCommands is the command listing half of AI.md PART 24
// "Service Help Output", reproduced verbatim - including its column
// widths, which are NOT internally consistent between the bare
// subcommands (38-char field) and the --prefixed ones (39-char field).
// That inconsistency is in the spec itself and is preserved exactly, not
// "fixed".
const serviceHelpCommands = `Service management commands:

start                                 - Start the service
stop                                  - Stop the service
restart                               - Restart the service
reload                                - Reload configuration without restart
--install                              - Install, enable, and start service
--disable                              - Stop and disable service (keeps data)
--uninstall                            - Stop, disable, and remove everything (keeps binary)
`

// ServiceHelpText is the complete AI.md PART 24 "Service Help Output"
// block, verbatim, with the "Current status" section left in its
// placeholder form. It exists so tests can byte-compare against the spec
// exactly as written; runtime output uses RenderServiceHelp instead.
const ServiceHelpText = serviceHelpCommands + `
Current status:
  Service:    installed / not installed
  State:      running / stopped / disabled
  Auto-start: enabled / disabled
  PID:        {pid} (if running)
`

// Status reports the live state of the installed service.
type Status struct {
	// Installed reports whether the unit/service file exists.
	Installed bool
	// Running reports whether the service is currently active.
	Running bool
	// AutoStart reports whether the service is enabled to start on boot.
	AutoStart bool
	// PID is the running process ID. Only meaningful when Running is true.
	PID int
}

// RenderServiceHelp renders AI.md PART 24 "Service Help Output" with the
// "Current status" section resolved from st, in place of the spec's
// slash-separated placeholder values.
func RenderServiceHelp(st Status) string {
	installed := "not installed"
	if st.Installed {
		installed = "installed"
	}

	state := "stopped"
	switch {
	case st.Running:
		state = "running"
	case st.Installed && !st.AutoStart:
		state = "disabled"
	}

	autoStart := "disabled"
	if st.AutoStart {
		autoStart = "enabled"
	}

	out := serviceHelpCommands + "\nCurrent status:\n"
	out += fmt.Sprintf("  Service:    %s\n", installed)
	out += fmt.Sprintf("  State:      %s\n", state)
	out += fmt.Sprintf("  Auto-start: %s\n", autoStart)
	if st.Running {
		out += fmt.Sprintf("  PID:        %d\n", st.PID)
	}
	return out
}
