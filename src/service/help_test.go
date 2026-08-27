package service

import (
	"strings"
	"testing"
)

func TestServiceHelpTextVerbatim(t *testing.T) {
	want := `Service management commands:

start                                 - Start the service
stop                                  - Stop the service
restart                               - Restart the service
reload                                - Reload configuration without restart
--install                              - Install, enable, and start service
--disable                              - Stop and disable service (keeps data)
--uninstall                            - Stop, disable, and remove everything (keeps binary)

Current status:
  Service:    installed / not installed
  State:      running / stopped / disabled
  Auto-start: enabled / disabled
  PID:        {pid} (if running)
`
	if ServiceHelpText != want {
		t.Errorf("ServiceHelpText mismatch\ngot:\n%q\nwant:\n%q", ServiceHelpText, want)
	}
}

func TestRenderServiceHelpNotInstalled(t *testing.T) {
	got := RenderServiceHelp(Status{})
	if !containsAll(got, "Service:    not installed", "State:      stopped", "Auto-start: disabled") {
		t.Errorf("unexpected render for not-installed status:\n%s", got)
	}
	if containsAll(got, "PID:") {
		t.Errorf("stopped status should omit PID line:\n%s", got)
	}
}

func TestRenderServiceHelpInstalledDisabled(t *testing.T) {
	got := RenderServiceHelp(Status{Installed: true})
	if !containsAll(got, "Service:    installed", "State:      disabled", "Auto-start: disabled") {
		t.Errorf("unexpected render for installed-disabled status:\n%s", got)
	}
}

func TestRenderServiceHelpRunning(t *testing.T) {
	got := RenderServiceHelp(Status{Installed: true, Running: true, AutoStart: true, PID: 4242})
	if !containsAll(got, "Service:    installed", "State:      running", "Auto-start: enabled", "PID:        4242") {
		t.Errorf("unexpected render for running status:\n%s", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
