package service

import "testing"

func TestDetectEscalationMethodsElevatedShortCircuits(t *testing.T) {
	got, err := DetectEscalationMethods("linux", true, newFakePathLookup("sudo", "su", "pkexec", "doas"))
	if err != nil {
		t.Fatalf("DetectEscalationMethods: %v", err)
	}
	if len(got) != 1 || got[0] != MethodNone {
		t.Errorf("got %v, want [%v]", got, MethodNone)
	}
}

func TestDetectEscalationMethodsLinuxOrder(t *testing.T) {
	pl := newFakePathLookup("doas", "pkexec", "su", "sudo")
	got, err := DetectEscalationMethods("linux", false, pl)
	if err != nil {
		t.Fatalf("DetectEscalationMethods: %v", err)
	}
	want := []Method{MethodSudo, MethodSu, MethodPkexec, MethodDoas}
	assertMethodsEqual(t, got, want)
}

func TestDetectEscalationMethodsLinuxFiltersUnavailable(t *testing.T) {
	pl := newFakePathLookup("doas")
	got, err := DetectEscalationMethods("linux", false, pl)
	if err != nil {
		t.Fatalf("DetectEscalationMethods: %v", err)
	}
	assertMethodsEqual(t, got, []Method{MethodDoas})
}

func TestDetectEscalationMethodsDarwinOrder(t *testing.T) {
	pl := newFakePathLookup("osascript", "sudo")
	got, err := DetectEscalationMethods("darwin", false, pl)
	if err != nil {
		t.Fatalf("DetectEscalationMethods: %v", err)
	}
	assertMethodsEqual(t, got, []Method{MethodSudo, MethodOsascript})
}

func TestDetectEscalationMethodsBSDOrder(t *testing.T) {
	pl := newFakePathLookup("su", "sudo", "doas")
	cases := []string{"freebsd", "openbsd", "netbsd"}
	for _, goos := range cases {
		t.Run(goos, func(t *testing.T) {
			got, err := DetectEscalationMethods(goos, false, pl)
			if err != nil {
				t.Fatalf("DetectEscalationMethods(%s): %v", goos, err)
			}
			assertMethodsEqual(t, got, []Method{MethodDoas, MethodSudo, MethodSu})
		})
	}
}

func TestDetectEscalationMethodsWindows(t *testing.T) {
	got, err := DetectEscalationMethods("windows", false, newFakePathLookup())
	if err != nil {
		t.Fatalf("DetectEscalationMethods: %v", err)
	}
	assertMethodsEqual(t, got, []Method{MethodUAC, MethodRunas})
}

func TestDetectEscalationMethodsUnsupportedGOOS(t *testing.T) {
	if _, err := DetectEscalationMethods("plan9", false, newFakePathLookup()); err == nil {
		t.Fatal("expected error for unsupported GOOS")
	}
}

func TestDetectEscalationReturnsFirstAvailable(t *testing.T) {
	pl := newFakePathLookup("doas")
	got, err := DetectEscalation("linux", false, pl)
	if err != nil {
		t.Fatalf("DetectEscalation: %v", err)
	}
	if got != MethodDoas {
		t.Errorf("got %v, want %v", got, MethodDoas)
	}
}

func TestDetectEscalationErrorsWhenNothingAvailable(t *testing.T) {
	_, err := DetectEscalation("linux", false, newFakePathLookup())
	if err == nil {
		t.Fatal("expected an error when the user cannot escalate at all")
	}
}

func TestDetectEscalationElevatedReturnsNone(t *testing.T) {
	got, err := DetectEscalation("linux", true, newFakePathLookup())
	if err != nil {
		t.Fatalf("DetectEscalation: %v", err)
	}
	if got != MethodNone {
		t.Errorf("got %v, want %v", got, MethodNone)
	}
}

func TestEscalationCommand(t *testing.T) {
	cases := []struct {
		name   string
		method Method
		want   []string
	}{
		{"none", MethodNone, []string{"/usr/local/bin/redxt", "--service", "--install"}},
		{"sudo", MethodSudo, []string{"sudo", "/usr/local/bin/redxt", "--service", "--install"}},
		{"su", MethodSu, []string{"su", "-c", "'/usr/local/bin/redxt' '--service' '--install'"}},
		{"pkexec", MethodPkexec, []string{"pkexec", "/usr/local/bin/redxt", "--service", "--install"}},
		{"doas", MethodDoas, []string{"doas", "/usr/local/bin/redxt", "--service", "--install"}},
		{"runas", MethodRunas, []string{"runas", "/user:Administrator", "/usr/local/bin/redxt", "--service", "--install"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EscalationCommand(tc.method, "/usr/local/bin/redxt", []string{"--service", "--install"})
			if err != nil {
				t.Fatalf("EscalationCommand: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("arg %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestEscalationCommandOsascript(t *testing.T) {
	got, err := EscalationCommand(MethodOsascript, "/usr/local/bin/redxt", []string{"--service", "--install"})
	if err != nil {
		t.Fatalf("EscalationCommand: %v", err)
	}
	if len(got) != 3 || got[0] != "osascript" || got[1] != "-e" {
		t.Fatalf("got %v, want osascript -e <script>", got)
	}
}

func TestEscalationCommandUnsupported(t *testing.T) {
	if _, err := EscalationCommand(Method("bogus"), "/bin/x", nil); err == nil {
		t.Fatal("expected error for unsupported method")
	}
}

func assertMethodsEqual(t *testing.T, got, want []Method) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
