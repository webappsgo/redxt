package service

import (
	"fmt"
	"io"
)

// Dispatch runs the `--service` subcommand named by args[0] (one of
// start, stop, restart, reload, --install, --disable, --uninstall,
// --help/help) and writes any operator-facing output to out. It is the
// single entrypoint src/cli wires to `{project_name} --service ...`.
func (m *Manager) Dispatch(args []string, out io.Writer) error {
	if len(args) == 0 {
		return m.writeHelp(out)
	}

	switch args[0] {
	case "start":
		return m.Start()
	case "stop":
		return m.Stop()
	case "restart":
		return m.Restart()
	case "reload":
		return m.Reload()
	case "--install":
		return m.Install()
	case "--disable":
		return m.Disable()
	case "--uninstall":
		result, err := m.Uninstall()
		if err != nil {
			return err
		}
		fmt.Fprintln(out, result.Message)
		return nil
	case "--help", "help":
		return m.writeHelp(out)
	default:
		return fmt.Errorf("service: unknown subcommand %q", args[0])
	}
}

// writeHelp renders AI.md PART 24 "Service Help Output" with the live
// status filled in.
func (m *Manager) writeHelp(out io.Writer) error {
	st, err := m.Status()
	if err != nil {
		// Status is best-effort for --help - an undetectable init system
		// should not block showing the command listing.
		st = Status{}
	}
	_, err = fmt.Fprint(out, RenderServiceHelp(st))
	return err
}
