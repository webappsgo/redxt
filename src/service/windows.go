package service

import "fmt"

// winInstall, winUninstall, winStart, winStop, and winStatus are the
// Windows Service Control Manager operations. They are package-level
// variables, not build-tagged methods, so Manager's dispatch logic stays
// build-tag-free: windows_install.go (built only on GOOS=windows, where
// golang.org/x/sys/windows/svc/mgr actually compiles) overwrites them via
// an init() function. On every other platform they return this error,
// which only matters if a non-Windows binary is somehow asked to manage a
// Windows service.
var (
	winInstall = func(Context) error {
		return fmt.Errorf("service: Windows service management is only available when built for GOOS=windows")
	}
	winUninstall = func(Context) error {
		return fmt.Errorf("service: Windows service management is only available when built for GOOS=windows")
	}
	winStart = func(name string) error {
		return fmt.Errorf("service: Windows service management is only available when built for GOOS=windows")
	}
	winStop = func(name string) error {
		return fmt.Errorf("service: Windows service management is only available when built for GOOS=windows")
	}
	winStatus = func(name string) (Status, error) {
		return Status{}, fmt.Errorf("service: Windows service management is only available when built for GOOS=windows")
	}
)
