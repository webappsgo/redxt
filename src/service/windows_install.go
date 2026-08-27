//go:build windows

package service

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// init wires the winInstall/winUninstall/winStart/winStop/winStatus
// package-level seams (declared in windows.go) to their real
// implementations. It only runs when built for GOOS=windows, since
// golang.org/x/sys/windows/svc/mgr only compiles on that platform.
func init() {
	winInstall = doWindowsInstall
	winUninstall = doWindowsUninstall
	winStart = doWindowsStart
	winStop = doWindowsStop
	winStatus = doWindowsStatus
}

// doWindowsInstall creates the Windows service running as the
// auto-managed Virtual Service Account (empty ServiceStartName), then
// starts it. AI.md PART 24 "Go Implementation (Windows)".
func doWindowsInstall(ctx Context) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service: connecting to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ctx.InternalName)
	if err == nil {
		s.Close()
	} else {
		s, err = m.CreateService(ctx.InternalName, ctx.BinaryPath, mgr.Config{
			DisplayName: ctx.ProjectName,
			Description: ctx.ProjectName + " service",
			StartType:   mgr.StartAutomatic,
			// Empty = Virtual Service Account (NT SERVICE\{internal_name}).
			ServiceStartName: "",
		})
		if err != nil {
			return fmt.Errorf("service: creating service %s: %w", ctx.InternalName, err)
		}
		defer s.Close()
	}

	if err := s.Start(); err != nil {
		return fmt.Errorf("service: starting service %s: %w", ctx.InternalName, err)
	}
	return nil
}

// doWindowsUninstall stops and deletes the Windows service.
func doWindowsUninstall(ctx Context) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service: connecting to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ctx.InternalName)
	if err != nil {
		return fmt.Errorf("service: opening service %s: %w", ctx.InternalName, err)
	}
	defer s.Close()

	_, _ = s.Control(svc.Stop)
	if err := s.Delete(); err != nil {
		return fmt.Errorf("service: deleting service %s: %w", ctx.InternalName, err)
	}
	return nil
}

// doWindowsStart starts the named Windows service.
func doWindowsStart(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service: connecting to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("service: opening service %s: %w", name, err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("service: starting service %s: %w", name, err)
	}
	return nil
}

// doWindowsStop stops the named Windows service and waits briefly for it
// to leave the Running state.
func doWindowsStop(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service: connecting to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("service: opening service %s: %w", name, err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("service: stopping service %s: %w", name, err)
	}
	for i := 0; i < 30 && status.State != svc.Stopped; i++ {
		time.Sleep(time.Second)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("service: querying service %s: %w", name, err)
		}
	}
	return nil
}

// doWindowsStatus reports whether the named Windows service is installed,
// running, and set to auto-start.
func doWindowsStatus(name string) (Status, error) {
	m, err := mgr.Connect()
	if err != nil {
		return Status{}, fmt.Errorf("service: connecting to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		// ERROR_SERVICE_DOES_NOT_EXIST - not installed, not an error.
		return Status{}, nil
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return Status{}, fmt.Errorf("service: querying service %s: %w", name, err)
	}
	cfg, err := s.Config()
	if err != nil {
		return Status{}, fmt.Errorf("service: reading config for service %s: %w", name, err)
	}

	return Status{
		Installed: true,
		Running:   status.State == svc.Running,
		AutoStart: cfg.StartType == mgr.StartAutomatic,
		PID:       int(status.ProcessId),
	}, nil
}
