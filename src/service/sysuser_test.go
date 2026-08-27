package service

import (
	"testing"
)

func TestFindAvailableSystemIDSkipsReservedAndTaken(t *testing.T) {
	lookup := newFakeIDLookup()
	// Take everything from 899 down to 900-reserved-range boundary so the
	// first non-reserved, non-taken slot is forced further down.
	for id := 899; id >= 850; id-- {
		lookup.takenUIDs[id] = true
	}
	got, err := FindAvailableLinuxSystemID(lookup)
	if err != nil {
		t.Fatalf("FindAvailableLinuxSystemID: %v", err)
	}
	if got != 849 {
		t.Errorf("got %d, want 849 (first free id below the taken block)", got)
	}
	if reservedIDs[got] {
		t.Errorf("returned reserved id %d", got)
	}
}

func TestFindAvailableSystemIDSkipsReservedIDsExactly(t *testing.T) {
	lookup := newFakeIDLookup()
	got, err := FindAvailableLinuxSystemID(lookup)
	if err != nil {
		t.Fatalf("FindAvailableLinuxSystemID: %v", err)
	}
	if got != 899 {
		t.Fatalf("got %d, want 899 (top of range, unreserved and untaken)", got)
	}

	lookup.takenUIDs[899] = true
	got, err = FindAvailableLinuxSystemID(lookup)
	if err != nil {
		t.Fatalf("FindAvailableLinuxSystemID: %v", err)
	}
	if got != 898 {
		t.Fatalf("got %d, want 898", got)
	}
}

func TestFindAvailableSystemIDSkipsGIDTaken(t *testing.T) {
	lookup := newFakeIDLookup()
	lookup.takenGIDs[899] = true
	got, err := FindAvailableLinuxSystemID(lookup)
	if err != nil {
		t.Fatalf("FindAvailableLinuxSystemID: %v", err)
	}
	if got != 898 {
		t.Errorf("got %d, want 898 (899's gid is taken)", got)
	}
}

func TestFindAvailableSystemIDReservedTableNeverReturned(t *testing.T) {
	lookup := newFakeIDLookup()
	for id := range reservedIDs {
		lookup.takenUIDs[id] = false
		lookup.takenGIDs[id] = false
	}
	id, err := FindAvailableLinuxSystemID(lookup)
	if err != nil {
		t.Fatalf("FindAvailableLinuxSystemID: %v", err)
	}
	if reservedIDs[id] {
		t.Errorf("returned a reserved id: %d", id)
	}
}

func TestFindAvailableSystemIDExhaustsRange(t *testing.T) {
	lookup := newFakeIDLookup()
	for id := LinuxIDRangeBottom; id <= LinuxIDRangeTop; id++ {
		lookup.takenUIDs[id] = true
	}
	_, err := FindAvailableLinuxSystemID(lookup)
	if err == nil {
		t.Fatal("expected an error when the entire range is taken")
	}
}

func TestFindAvailableMacOSSystemIDUsesNarrowerRange(t *testing.T) {
	lookup := newFakeIDLookup()
	got, err := FindAvailableMacOSSystemID(lookup)
	if err != nil {
		t.Fatalf("FindAvailableMacOSSystemID: %v", err)
	}
	if got != DarwinIDRangeTop {
		t.Errorf("got %d, want %d", got, DarwinIDRangeTop)
	}
	if got > 399 {
		t.Errorf("id %d exceeds macOS safe range top of 399", got)
	}
}

func TestNologinShellPrefersSbin(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{"both present", []string{"/sbin/nologin", "/usr/sbin/nologin"}, "/sbin/nologin"},
		{"only sbin", []string{"/sbin/nologin"}, "/sbin/nologin"},
		{"only usr-sbin", []string{"/usr/sbin/nologin"}, "/usr/sbin/nologin"},
		{"neither", nil, "/sbin/nologin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NologinShell(newFakeFileLookup(tc.files...))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLinuxCreateCommandsVerbatim(t *testing.T) {
	got := LinuxCreateCommands("redxt", 512, "/etc/webappsgo/redxt", "/sbin/nologin")
	want := [][]string{
		{"groupadd", "--system", "--gid", "512", "redxt"},
		{
			"useradd", "--system",
			"--uid", "512",
			"--gid", "512",
			"--home-dir", "/etc/webappsgo/redxt",
			"--shell", "/sbin/nologin",
			"--comment", "redxt service account",
			"redxt",
		},
	}
	assertCommandsEqual(t, got, want)
}

func TestDarwinCreateCommandsVerbatim(t *testing.T) {
	got := DarwinCreateCommands("redxt", 300, "/usr/local/var/webappsgo/redxt")
	want := [][]string{
		{"dscl", ".", "-create", "/Groups/redxt"},
		{"dscl", ".", "-create", "/Groups/redxt", "PrimaryGroupID", "300"},
		{"dscl", ".", "-create", "/Groups/redxt", "Password", "*"},
		{"dscl", ".", "-create", "/Users/redxt"},
		{"dscl", ".", "-create", "/Users/redxt", "UniqueID", "300"},
		{"dscl", ".", "-create", "/Users/redxt", "PrimaryGroupID", "300"},
		{"dscl", ".", "-create", "/Users/redxt", "UserShell", "/usr/bin/false"},
		{"dscl", ".", "-create", "/Users/redxt", "RealName", "redxt service account"},
		{"dscl", ".", "-create", "/Users/redxt", "NFSHomeDirectory", "/usr/local/var/webappsgo/redxt"},
		{"dscl", ".", "-create", "/Users/redxt", "Password", "*"},
		{"dscl", ".", "-create", "/Users/redxt", "IsHidden", "1"},
	}
	assertCommandsEqual(t, got, want)
}

func TestFreeBSDCreateCommandsVerbatim(t *testing.T) {
	got := FreeBSDCreateCommands("redxt", 250, "/var/lib/webappsgo/redxt")
	want := [][]string{
		{"pw", "groupadd", "-n", "redxt", "-g", "250"},
		{
			"pw", "useradd",
			"-n", "redxt",
			"-u", "250",
			"-g", "250",
			"-d", "/var/lib/webappsgo/redxt",
			"-s", "/usr/sbin/nologin",
			"-c", "redxt service account",
		},
	}
	assertCommandsEqual(t, got, want)
}

func TestCreateCommandsWindowsIsNoop(t *testing.T) {
	got, err := CreateCommands("windows", "redxt", 1, "C:\\ProgramData", newFakeFileLookup())
	if err != nil {
		t.Fatalf("CreateCommands: %v", err)
	}
	if got != nil {
		t.Errorf("windows should create no commands, got %v", got)
	}
}

func TestCreateCommandsUnsupportedGOOS(t *testing.T) {
	if _, err := CreateCommands("plan9", "redxt", 1, "/", newFakeFileLookup()); err == nil {
		t.Fatal("expected error for unsupported GOOS")
	}
}

func assertCommandsEqual(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d commands, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("command %d: got %v, want %v", i, got[i], want[i])
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("command %d arg %d: got %q, want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}
