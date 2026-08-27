package service

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/webappsgo/redxt/src/pidfile"
)

// unitFile is one file Install must write to disk.
type unitFile struct {
	Path       string
	Content    string
	Executable bool
}

// unitFiles returns the file(s) init requires, rendered from m.Ctx.
// InitWindowsSCM has none - the Windows Service Control Manager stores
// the service definition in the registry, not a file.
func (m *Manager) unitFiles(init InitSystem) []unitFile {
	name := m.Ctx.InternalName
	plist := m.Ctx.plistName()
	switch init {
	case InitSystemd:
		return []unitFile{{UnitPath(init, m.Root, name, plist), RenderSystemd(m.Ctx), false}}
	case InitOpenRC:
		return []unitFile{{UnitPath(init, m.Root, name, plist), RenderOpenRC(m.Ctx), true}}
	case InitSysVinit:
		return []unitFile{{UnitPath(init, m.Root, name, plist), RenderSysVinit(m.Ctx), true}}
	case InitRunit:
		run, logRun := RunitPaths(m.Root, name)
		return []unitFile{
			{run, RenderRunitRun(m.Ctx), true},
			{logRun, RenderRunitLogRun(m.Ctx), true},
		}
	case InitRcd:
		return []unitFile{{UnitPath(init, m.Root, name, plist), RenderRcd(m.Ctx), true}}
	case InitLaunchd:
		return []unitFile{{UnitPath(init, m.Root, name, plist), RenderLaunchd(m.Ctx), false}}
	default:
		return nil
	}
}

// initCommands are the external commands that drive one init system's
// service lifecycle.
type initCommands struct {
	Start     []string
	Stop      []string
	Restart   []string
	Reload    []string
	Enable    []string
	Disable   []string
	IsEnabled []string
}

// commandsFor builds the lifecycle commands for init, given the primary
// unit file path already resolved by unitFiles.
func (m *Manager) commandsFor(init InitSystem, unitPath string) initCommands {
	name := m.Ctx.InternalName
	switch init {
	case InitSystemd:
		return initCommands{
			Start:     []string{"systemctl", "start", name},
			Stop:      []string{"systemctl", "stop", name},
			Restart:   []string{"systemctl", "restart", name},
			Reload:    []string{"systemctl", "reload", name},
			Enable:    []string{"systemctl", "enable", name},
			Disable:   []string{"systemctl", "disable", name},
			IsEnabled: []string{"systemctl", "is-enabled", name},
		}
	case InitOpenRC:
		return initCommands{
			Start:   []string{"rc-service", name, "start"},
			Stop:    []string{"rc-service", name, "stop"},
			Restart: []string{"rc-service", name, "restart"},
			// OpenRC has no dedicated reload action; restart stands in.
			Reload:    []string{"rc-service", name, "restart"},
			Enable:    []string{"rc-update", "add", name, "default"},
			Disable:   []string{"rc-update", "del", name, "default"},
			IsEnabled: []string{"rc-update", "show", "default"},
		}
	case InitSysVinit:
		return initCommands{
			Start:   []string{unitPath, "start"},
			Stop:    []string{unitPath, "stop"},
			Restart: []string{unitPath, "restart"},
			// The AI.md PART 25 SysVinit script only supports
			// start/stop/restart/status; restart stands in for reload.
			Reload:    []string{unitPath, "restart"},
			Enable:    []string{"update-rc.d", name, "defaults"},
			Disable:   []string{"update-rc.d", "-f", name, "remove"},
			IsEnabled: []string{"update-rc.d", name, "defaults"},
		}
	case InitRunit:
		svcDir := filepath.Dir(unitPath)
		return initCommands{
			Start:     []string{"sv", "up", svcDir},
			Stop:      []string{"sv", "down", svcDir},
			Restart:   []string{"sv", "restart", svcDir},
			Reload:    []string{"sv", "reload", svcDir},
			Enable:    []string{"ln", "-sf", svcDir, "/etc/service/" + name},
			Disable:   []string{"rm", "-f", "/etc/service/" + name},
			IsEnabled: []string{"test", "-L", "/etc/service/" + name},
		}
	case InitRcd:
		return initCommands{
			Start:     []string{"service", name, "start"},
			Stop:      []string{"service", name, "stop"},
			Restart:   []string{"service", name, "restart"},
			Reload:    []string{"service", name, "reload"},
			Enable:    []string{"sysrc", name + "_enable=YES"},
			Disable:   []string{"sysrc", name + "_enable=NO"},
			IsEnabled: []string{"sysrc", "-n", name + "_enable"},
		}
	case InitLaunchd:
		return initCommands{
			Start:     []string{"launchctl", "load", unitPath},
			Stop:      []string{"launchctl", "unload", unitPath},
			Enable:    []string{"launchctl", "load", unitPath},
			Disable:   []string{"launchctl", "unload", unitPath},
			IsEnabled: []string{"launchctl", "list", name},
		}
	default:
		return initCommands{}
	}
}

// writeUnitFiles writes every file in files to disk, creating parent
// directories as needed and marking scripts executable per AI.md PART 25.
func writeUnitFiles(files []unitFile) error {
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return fmt.Errorf("service: creating %s: %w", filepath.Dir(f.Path), err)
		}
		mode := os.FileMode(0o644)
		if f.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(f.Path, []byte(f.Content), mode); err != nil {
			return fmt.Errorf("service: writing %s: %w", f.Path, err)
		}
	}
	return nil
}

// Install writes the service unit file(s), enables the service for
// auto-start, and starts it. Per AI.md PART 24 "Service Installation
// Logic", it does NOT create the {internal_name} user or set up
// directories - that happens during normal startup, not here.
func (m *Manager) Install() error {
	init, err := m.detectInit()
	if err != nil {
		return err
	}

	if init == InitWindowsSCM {
		return winInstall(m.Ctx)
	}

	files := m.unitFiles(init)
	if len(files) == 0 {
		return fmt.Errorf("service: no unit files defined for init system %q", init)
	}
	if err := writeUnitFiles(files); err != nil {
		return err
	}

	cmds := m.commandsFor(init, files[0].Path)
	if len(cmds.Enable) > 0 {
		if err := m.Runner.Run(cmds.Enable[0], cmds.Enable[1:]...); err != nil {
			return fmt.Errorf("service: enabling %s: %w", m.Ctx.InternalName, err)
		}
	}
	if err := m.Runner.Run(cmds.Start[0], cmds.Start[1:]...); err != nil {
		return fmt.Errorf("service: starting %s: %w", m.Ctx.InternalName, err)
	}
	m.logf("info", "service: installed and started %s (%s)", m.Ctx.InternalName, init)
	return nil
}

// uninstallConfirmPrompt is verbatim per AI.md PART 24 "Service Uninstall
// Logic".
const uninstallConfirmPrompt = "This will delete ALL data, configs, and the system user. Continue? [y/N]"

// UninstallResult carries the message Uninstall wants printed to the
// operator, matching AI.md PART 24's exact wording.
type UninstallResult struct {
	// Message is "Service uninstalled. Delete binary manually: rm {binary_path}"
	// on success.
	Message string
}

// ErrUninstallAborted is returned when the operator declines the
// destructive-action confirmation prompt.
var ErrUninstallAborted = errors.New("service: uninstall aborted by operator")

// Uninstall stops and disables the service, removes its unit file(s),
// deletes every data directory, and deletes the system user/group if this
// host has one named {internal_name}. It requires m.Confirm to return true
// for AI.md PART 24's confirmation prompt before doing anything
// destructive.
func (m *Manager) Uninstall() (UninstallResult, error) {
	confirm := m.Confirm
	if confirm == nil {
		confirm = defaultConfirm
	}
	if !confirm(uninstallConfirmPrompt) {
		return UninstallResult{}, ErrUninstallAborted
	}

	init, err := m.detectInit()
	if err != nil {
		return UninstallResult{}, err
	}

	if init == InitWindowsSCM {
		if err := winUninstall(m.Ctx); err != nil {
			return UninstallResult{}, err
		}
	} else {
		files := m.unitFiles(init)
		if len(files) > 0 {
			cmds := m.commandsFor(init, files[0].Path)
			// Stop/disable failures are not fatal - the service may
			// already be stopped or the unit file already gone.
			if len(cmds.Stop) > 0 {
				_ = m.Runner.Run(cmds.Stop[0], cmds.Stop[1:]...)
			}
			if len(cmds.Disable) > 0 {
				_ = m.Runner.Run(cmds.Disable[0], cmds.Disable[1:]...)
			}
		}
		for _, f := range files {
			if err := os.RemoveAll(f.Path); err != nil && !os.IsNotExist(err) {
				return UninstallResult{}, fmt.Errorf("service: removing %s: %w", f.Path, err)
			}
		}
		if init == InitRunit {
			_ = os.RemoveAll(filepath.Dir(files[0].Path))
		}
	}

	for _, dir := range []string{m.Ctx.ConfigDir, m.Ctx.DataDir, m.Ctx.CacheDir, m.Ctx.LogDir, m.Ctx.BackupDir} {
		if dir == "" {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return UninstallResult{}, fmt.Errorf("service: removing %s: %w", dir, err)
		}
	}
	if m.Ctx.PIDFile != "" {
		if err := os.Remove(m.Ctx.PIDFile); err != nil && !os.IsNotExist(err) {
			return UninstallResult{}, fmt.Errorf("service: removing %s: %w", m.Ctx.PIDFile, err)
		}
	}

	if err := m.deleteSystemUser(); err != nil {
		m.logf("warn", "service: deleting system user %s: %v", m.Ctx.InternalName, err)
	}

	return UninstallResult{
		Message: fmt.Sprintf("Service uninstalled. Delete binary manually: rm %s", m.Ctx.BinaryPath),
	}, nil
}

// deleteSystemUser removes the {internal_name} system user/group, if one
// exists. redxt is the only owner of an account named {internal_name}, so
// existence is treated as "created by server" per AI.md PART 24's
// "Delete system user and group (if created by server)" step. Windows'
// Virtual Service Account is auto-managed and needs no deletion.
func (m *Manager) deleteSystemUser() error {
	if m.GOOS == "windows" {
		return nil
	}
	name := m.Ctx.InternalName
	if _, err := user.Lookup(name); err != nil {
		return nil
	}
	var commands [][]string
	switch m.GOOS {
	case "linux":
		commands = [][]string{{"userdel", name}, {"groupdel", name}}
	case "darwin":
		commands = [][]string{{"dscl", ".", "-delete", "/Users/" + name}, {"dscl", ".", "-delete", "/Groups/" + name}}
	case "freebsd", "openbsd", "netbsd":
		commands = [][]string{{"pw", "userdel", "-n", name}, {"pw", "groupdel", "-n", name}}
	default:
		return fmt.Errorf("service: unsupported GOOS %q for system user deletion", m.GOOS)
	}
	return RunAll(m.Runner, commands)
}

// Disable stops the service and removes it from auto-start, keeping the
// unit file, all data, and the system user in place.
func (m *Manager) Disable() error {
	init, err := m.detectInit()
	if err != nil {
		return err
	}
	if init == InitWindowsSCM {
		return winStop(m.Ctx.InternalName)
	}
	files := m.unitFiles(init)
	if len(files) == 0 {
		return fmt.Errorf("service: no unit files defined for init system %q", init)
	}
	cmds := m.commandsFor(init, files[0].Path)
	if err := m.Runner.Run(cmds.Stop[0], cmds.Stop[1:]...); err != nil {
		return fmt.Errorf("service: stopping %s: %w", m.Ctx.InternalName, err)
	}
	if len(cmds.Disable) > 0 {
		if err := m.Runner.Run(cmds.Disable[0], cmds.Disable[1:]...); err != nil {
			return fmt.Errorf("service: disabling %s: %w", m.Ctx.InternalName, err)
		}
	}
	return nil
}

// Start starts the service via the detected init system.
func (m *Manager) Start() error { return m.runLifecycle("start") }

// Stop stops the service via the detected init system.
func (m *Manager) Stop() error { return m.runLifecycle("stop") }

// Restart restarts the service via the detected init system.
func (m *Manager) Restart() error { return m.runLifecycle("restart") }

// Reload asks the service to reload its configuration without a full
// restart, via the detected init system.
func (m *Manager) Reload() error { return m.runLifecycle("reload") }

// runLifecycle dispatches one of start/stop/restart/reload to the
// detected init system.
func (m *Manager) runLifecycle(action string) error {
	init, err := m.detectInit()
	if err != nil {
		return err
	}
	if init == InitWindowsSCM {
		switch action {
		case "start":
			return winStart(m.Ctx.InternalName)
		case "stop":
			return winStop(m.Ctx.InternalName)
		case "restart":
			if err := winStop(m.Ctx.InternalName); err != nil {
				return err
			}
			return winStart(m.Ctx.InternalName)
		case "reload":
			// The Windows SCM has no reload signal; restart stands in.
			if err := winStop(m.Ctx.InternalName); err != nil {
				return err
			}
			return winStart(m.Ctx.InternalName)
		}
	}

	files := m.unitFiles(init)
	if len(files) == 0 {
		return fmt.Errorf("service: no unit files defined for init system %q", init)
	}
	cmds := m.commandsFor(init, files[0].Path)

	var cmd []string
	switch action {
	case "start":
		cmd = cmds.Start
	case "stop":
		cmd = cmds.Stop
	case "restart":
		cmd = cmds.Restart
		if len(cmd) == 0 {
			// launchd has no restart verb; unload then load.
			if err := m.Runner.Run(cmds.Stop[0], cmds.Stop[1:]...); err != nil {
				return err
			}
			cmd = cmds.Start
		}
	case "reload":
		cmd = cmds.Reload
		if len(cmd) == 0 {
			cmd = cmds.Restart
		}
		if len(cmd) == 0 {
			if err := m.Runner.Run(cmds.Stop[0], cmds.Stop[1:]...); err != nil {
				return err
			}
			cmd = cmds.Start
		}
	default:
		return fmt.Errorf("service: unknown lifecycle action %q", action)
	}
	if err := m.Runner.Run(cmd[0], cmd[1:]...); err != nil {
		return fmt.Errorf("service: %s %s: %w", action, m.Ctx.InternalName, err)
	}
	return nil
}

// Status reports whether the service is installed, running, and set to
// auto-start, reading the running PID from m.Ctx.PIDFile via
// src/pidfile - the same file redxt itself writes on startup.
func (m *Manager) Status() (Status, error) {
	init, err := m.detectInit()
	if err != nil {
		return Status{}, err
	}
	if init == InitWindowsSCM {
		return winStatus(m.Ctx.InternalName)
	}

	files := m.unitFiles(init)
	installed := len(files) > 0
	for _, f := range files {
		if !m.FileLU.Exists(f.Path) {
			installed = false
			break
		}
	}

	running, pid := false, 0
	if m.Ctx.PIDFile != "" {
		if isRunning, foundPID, err := pidfile.Check(m.Ctx.PIDFile); err == nil {
			running, pid = isRunning, foundPID
		}
	}

	autoStart := false
	if installed && len(files) > 0 {
		cmds := m.commandsFor(init, files[0].Path)
		if len(cmds.IsEnabled) > 0 {
			if err := m.Runner.Run(cmds.IsEnabled[0], cmds.IsEnabled[1:]...); err == nil {
				autoStart = true
			}
		}
	}

	return Status{Installed: installed, Running: running, AutoStart: autoStart, PID: pid}, nil
}
