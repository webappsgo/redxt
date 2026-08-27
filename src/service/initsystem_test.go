package service

import "testing"

func TestDetectInitLinuxSystemd(t *testing.T) {
	fl := newFakeFileLookup("/run/systemd/system")
	pl := newFakePathLookup("systemctl")
	got, err := DetectInit("linux", fl, pl)
	if err != nil {
		t.Fatalf("DetectInit: %v", err)
	}
	if got != InitSystemd {
		t.Errorf("got %v, want %v", got, InitSystemd)
	}
}

func TestDetectInitLinuxOpenRC(t *testing.T) {
	fl := newFakeFileLookup("/sbin/openrc-run")
	pl := newFakePathLookup()
	got, err := DetectInit("linux", fl, pl)
	if err != nil {
		t.Fatalf("DetectInit: %v", err)
	}
	if got != InitOpenRC {
		t.Errorf("got %v, want %v", got, InitOpenRC)
	}
}

func TestDetectInitLinuxRunit(t *testing.T) {
	fl := newFakeFileLookup("/etc/sv")
	pl := newFakePathLookup("sv")
	got, err := DetectInit("linux", fl, pl)
	if err != nil {
		t.Fatalf("DetectInit: %v", err)
	}
	if got != InitRunit {
		t.Errorf("got %v, want %v", got, InitRunit)
	}
}

func TestDetectInitLinuxSysVinit(t *testing.T) {
	fl := newFakeFileLookup("/etc/init.d")
	pl := newFakePathLookup("update-rc.d")
	got, err := DetectInit("linux", fl, pl)
	if err != nil {
		t.Fatalf("DetectInit: %v", err)
	}
	if got != InitSysVinit {
		t.Errorf("got %v, want %v", got, InitSysVinit)
	}
}

func TestDetectInitLinuxSysVinitViaChkconfig(t *testing.T) {
	fl := newFakeFileLookup("/etc/init.d")
	pl := newFakePathLookup("chkconfig")
	got, err := DetectInit("linux", fl, pl)
	if err != nil {
		t.Fatalf("DetectInit: %v", err)
	}
	if got != InitSysVinit {
		t.Errorf("got %v, want %v", got, InitSysVinit)
	}
}

func TestDetectInitLinuxNoneDetected(t *testing.T) {
	fl := newFakeFileLookup()
	pl := newFakePathLookup()
	if _, err := DetectInit("linux", fl, pl); err == nil {
		t.Fatal("expected an error when no init system is detected")
	}
}

func TestDetectInitLinuxSystemdTakesPriorityOverOpenRC(t *testing.T) {
	fl := newFakeFileLookup("/run/systemd/system", "/sbin/openrc-run")
	pl := newFakePathLookup("systemctl")
	got, err := DetectInit("linux", fl, pl)
	if err != nil {
		t.Fatalf("DetectInit: %v", err)
	}
	if got != InitSystemd {
		t.Errorf("got %v, want %v (systemd must win when both are present)", got, InitSystemd)
	}
}

func TestDetectInitOtherPlatforms(t *testing.T) {
	fl := newFakeFileLookup()
	pl := newFakePathLookup()
	cases := []struct {
		goos string
		want InitSystem
	}{
		{"darwin", InitLaunchd},
		{"freebsd", InitRcd},
		{"openbsd", InitRcd},
		{"windows", InitWindowsSCM},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			got, err := DetectInit(tc.goos, fl, pl)
			if err != nil {
				t.Fatalf("DetectInit(%s): %v", tc.goos, err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectInitUnsupportedGOOS(t *testing.T) {
	if _, err := DetectInit("plan9", newFakeFileLookup(), newFakePathLookup()); err == nil {
		t.Fatal("expected error for unsupported GOOS")
	}
}

func TestUnitPathAndRunitPaths(t *testing.T) {
	root := "/tmproot"
	if got, want := UnitPath(InitSystemd, root, "redxt", ""), root+"/etc/systemd/system/redxt.service"; got != want {
		t.Errorf("systemd path: got %q, want %q", got, want)
	}
	if got, want := UnitPath(InitOpenRC, root, "redxt", ""), root+"/etc/init.d/redxt"; got != want {
		t.Errorf("openrc path: got %q, want %q", got, want)
	}
	if got, want := UnitPath(InitSysVinit, root, "redxt", ""), root+"/etc/init.d/redxt"; got != want {
		t.Errorf("sysvinit path: got %q, want %q", got, want)
	}
	if got, want := UnitPath(InitRcd, root, "redxt", ""), root+"/usr/local/etc/rc.d/redxt"; got != want {
		t.Errorf("rc.d path: got %q, want %q", got, want)
	}
	if got, want := UnitPath(InitLaunchd, root, "redxt", "org.webappsgo.redxt"), root+"/Library/LaunchDaemons/org.webappsgo.redxt.plist"; got != want {
		t.Errorf("launchd path: got %q, want %q", got, want)
	}

	run, logRun := RunitPaths(root, "redxt")
	if want := root + "/etc/sv/redxt/run"; run != want {
		t.Errorf("runit run: got %q, want %q", run, want)
	}
	if want := root + "/etc/sv/redxt/log/run"; logRun != want {
		t.Errorf("runit log/run: got %q, want %q", logRun, want)
	}
}
