package update

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// restartService restarts serviceName via the platform's service
// manager, per AI.md PART 23 "Service-Aware Update". When no service
// manager is present, or the restart command fails, it falls back to
// re-executing this process directly (restartSelf) — the same outcome
// a non-service invocation of --update yes produces.
func restartService(serviceName string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = restartLinuxService(serviceName)
	case "darwin":
		err = restartDarwinService(serviceName)
	case "freebsd", "openbsd", "netbsd":
		err = restartBSDService(serviceName)
	case "windows":
		err = restartWindowsService(serviceName)
	default:
		err = fmt.Errorf("update: unsupported platform: %s", runtime.GOOS)
	}
	if err != nil {
		return restartSelf()
	}
	return nil
}

// restartLinuxService tries systemd first, falling back to the generic
// SysV "service" command used by runit/s6/OpenRC-style init systems.
func restartLinuxService(serviceName string) error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		if err := exec.Command("systemctl", "restart", serviceName).Run(); err == nil {
			return nil
		}
	}
	return exec.Command("service", serviceName, "restart").Run()
}

// restartDarwinService uses "kickstart -k" to kill and restart the
// launchd job in one step. serviceName is the full launchd label this
// build was installed under (e.g. "com.webappsgo.redxt").
func restartDarwinService(serviceName string) error {
	return exec.Command("launchctl", "kickstart", "-k", "system/"+serviceName).Run()
}

// restartBSDService restarts an rc.d-managed service.
func restartBSDService(serviceName string) error {
	return exec.Command("service", serviceName, "restart").Run()
}

// restartWindowsService stops then starts an SCM-managed service. A
// failure to stop (e.g. already stopped) is ignored; only the start
// result is returned.
func restartWindowsService(serviceName string) error {
	exec.Command("sc", "stop", serviceName).Run()
	time.Sleep(2 * time.Second)
	return exec.Command("sc", "start", serviceName).Run()
}
