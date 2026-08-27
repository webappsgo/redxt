package overlay

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cretz/bine/tor"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/paths"
)

// withFakeBinaryFS swaps statFile/lookPath for the duration of a test.
func withFakeBinaryFS(t *testing.T, existing map[string]bool, pathBin string) {
	t.Helper()
	origStat, origLookup := statFile, lookPath
	statFile = func(name string) (os.FileInfo, error) {
		if existing[name] {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	lookPath = func(file string) (string, error) {
		if pathBin != "" {
			return pathBin, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() {
		statFile, lookPath = origStat, origLookup
	})
}

func TestResolveTorBinary(t *testing.T) {
	cases := []struct {
		name       string
		cfgBinary  string
		existing   map[string]bool
		pathBin    string
		wantBinary string
		wantErr    bool
	}{
		{
			name:       "explicit config path wins",
			cfgBinary:  "/custom/tor",
			existing:   map[string]bool{"/custom/tor": true, "/usr/bin/tor": true},
			wantBinary: "/custom/tor",
		},
		{
			name:      "explicit config path missing is an error, no fallback",
			cfgBinary: "/custom/tor",
			existing:  map[string]bool{"/usr/bin/tor": true},
			wantErr:   true,
		},
		{
			name:       "well-known path found",
			existing:   map[string]bool{"/usr/sbin/tor": true},
			wantBinary: "/usr/sbin/tor",
		},
		{
			name:       "falls back to $PATH",
			existing:   map[string]bool{},
			pathBin:    "/opt/bin/tor",
			wantBinary: "/opt/bin/tor",
		},
		{
			name:     "not found anywhere",
			existing: map[string]bool{},
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeBinaryFS(t, tc.existing, tc.pathBin)
			got, err := resolveTorBinary(&config.Tor{Binary: tc.cfgBinary})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got binary %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantBinary {
				t.Fatalf("got %q, want %q", got, tc.wantBinary)
			}
		})
	}
}

func baseTorConfig() *config.Tor {
	return &config.Tor{
		SafeLogging:          true,
		BandwidthRate:        "1 MB",
		BandwidthBurst:       "2 MB",
		MaxMonthlyBandwidth:  "unlimited",
		VirtualPort:          80,
		BootstrapTimeout:     60,
		MaxCircuits:          8,
		NumIntroPoints:       3,
		MaxStreamsPerCircuit: 10,
	}
}

func TestGetTorConfigDirectives(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*config.Tor)
		wantHas []string
		wantNot []string
	}{
		{
			name:   "network disabled: SocksPort 0",
			mutate: func(c *config.Tor) { c.UseNetwork, c.AllowUserPreference = false, false },
			wantHas: []string{
				"SocksPort 0",
				"ControlPort 127.0.0.1:auto",
				"SafeLogging 1",
				"ExitRelay 0",
				"ExitPolicy reject *:*",
				"ORPort 0",
				"DirPort 0",
				"HiddenServiceVersion 3",
				"HiddenServiceExportCircuitID haproxy",
			},
		},
		{
			name:    "network enabled: SocksPort auto",
			mutate:  func(c *config.Tor) { c.UseNetwork = true },
			wantHas: []string{"SocksPort auto"},
			wantNot: []string{"SocksPort 0"},
		},
		{
			name:    "user preference alone also enables SocksPort auto",
			mutate:  func(c *config.Tor) { c.AllowUserPreference = true },
			wantHas: []string{"SocksPort auto"},
		},
		{
			name:    "safe logging off",
			mutate:  func(c *config.Tor) { c.SafeLogging = false },
			wantHas: []string{"SafeLogging 0"},
			wantNot: []string{"SafeLogging 1"},
		},
		{
			name:    "unlimited bandwidth: no accounting block",
			mutate:  func(c *config.Tor) { c.MaxMonthlyBandwidth = "unlimited" },
			wantNot: []string{"AccountingStart", "AccountingMax"},
		},
		{
			name:    "accounting enabled with a real limit",
			mutate:  func(c *config.Tor) { c.MaxMonthlyBandwidth = "100 GB" },
			wantHas: []string{"AccountingStart month 1 00:00", "AccountingMax 100 GB"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseTorConfig()
			tc.mutate(cfg)
			got := getTorConfig(cfg, "/data/tor/site", 54321)
			for _, want := range tc.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("torrc missing directive %q\n---\n%s", want, got)
				}
			}
			for _, notWant := range tc.wantNot {
				if strings.Contains(got, notWant) {
					t.Errorf("torrc unexpectedly contains %q\n---\n%s", notWant, got)
				}
			}
			if !strings.Contains(got, "HiddenServiceDir /data/tor/site") {
				t.Errorf("torrc missing HiddenServiceDir")
			}
			if !strings.Contains(got, "HiddenServicePort 80 127.0.0.1:54321") {
				t.Errorf("torrc missing HiddenServicePort mapping")
			}
		})
	}
}

func TestShouldUseTor(t *testing.T) {
	cases := []struct {
		name                string
		useNetwork          bool
		allowUserPreference bool
		userPref            *bool
		want                bool
	}{
		{name: "no user pref allowed, server off", useNetwork: false, allowUserPreference: false, userPref: nil, want: false},
		{name: "no user pref allowed, server on", useNetwork: true, allowUserPreference: false, userPref: nil, want: true},
		{name: "user pref allowed but nil pref falls back to server off", useNetwork: false, allowUserPreference: true, userPref: nil, want: false},
		{name: "user pref allowed but nil pref falls back to server on", useNetwork: true, allowUserPreference: true, userPref: nil, want: true},
		{name: "user pref allowed, user says true overrides server off", useNetwork: false, allowUserPreference: true, userPref: boolPtr(true), want: true},
		{name: "user pref allowed, user says false overrides server on", useNetwork: true, allowUserPreference: true, userPref: boolPtr(false), want: false},
		{name: "user pref disallowed, user pref set is ignored", useNetwork: false, allowUserPreference: false, userPref: boolPtr(true), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Tor{UseNetwork: tc.useNetwork, AllowUserPreference: tc.allowUserPreference}
			got := ShouldUseTor(cfg, tc.userPref)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

func TestEnsureTorDirsPermissions(t *testing.T) {
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}

	if err := ensureTorDirs(p); err != nil {
		t.Fatalf("ensureTorDirs: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(p.Config, "tor"),
		filepath.Join(p.Data, "tor"),
		filepath.Join(p.Data, "tor", "site"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("dir %s has perm %o, want 0700", dir, perm)
		}
	}

	// Re-running must stay idempotent even if permissions drifted.
	loosened := filepath.Join(p.Data, "tor", "site")
	if err := os.Chmod(loosened, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := ensureTorDirs(p); err != nil {
		t.Fatalf("ensureTorDirs (second run): %v", err)
	}
	info, err := os.Stat(loosened)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir %s has perm %o after re-run, want 0700", loosened, perm)
	}
}

func TestUpdateTorrcPermissions(t *testing.T) {
	tmp := t.TempDir()
	torrcPath := filepath.Join(tmp, "config", "tor", "torrc")
	if err := updateTorrc(torrcPath, []byte("SocksPort 0\n")); err != nil {
		t.Fatalf("updateTorrc: %v", err)
	}
	info, err := os.Stat(torrcPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("torrc has perm %o, want 0600", perm)
	}
}

// fakeTorProcess is a scripted torProcess used to drive startDedicatedTor
// and the monitor loop without spawning a real tor binary.
type fakeTorProcess struct {
	enableErr  error
	healthErr  error
	closed     bool
	healthCall int
}

func (f *fakeTorProcess) EnableNetwork(context.Context, bool) error { return f.enableErr }
func (f *fakeTorProcess) Dialer(context.Context, *tor.DialConf) (*tor.Dialer, error) {
	return nil, errors.New("dialer not available in fake")
}
func (f *fakeTorProcess) Healthy() error {
	f.healthCall++
	return f.healthErr
}
func (f *fakeTorProcess) Close() error {
	f.closed = true
	return nil
}

func TestStartDedicatedTorNoBinary(t *testing.T) {
	withFakeBinaryFS(t, map[string]bool{}, "")
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseTorConfig()

	_, err := startDedicatedTor(context.Background(), cfg, p, nil, false)
	if err == nil {
		t.Fatal("expected error when no tor binary is available")
	}
}

func TestStartDedicatedTorSuccess(t *testing.T) {
	withFakeBinaryFS(t, map[string]bool{"/usr/bin/tor": true}, "")
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseTorConfig()

	hsDir := filepath.Join(p.Data, "tor", "site")
	if err := os.MkdirAll(hsDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hsDir, "hostname"), []byte("abcdefg.onion\n"), 0o600); err != nil {
		t.Fatalf("write hostname: %v", err)
	}

	fake := &fakeTorProcess{}
	origStart := startTorProcess
	startTorProcess = func(ctx context.Context, conf *tor.StartConf) (torProcess, error) {
		if conf.ExePath != "/usr/bin/tor" {
			t.Errorf("unexpected ExePath %q", conf.ExePath)
		}
		return fake, nil
	}
	t.Cleanup(func() { startTorProcess = origStart })

	svc, err := startDedicatedTor(context.Background(), cfg, p, nil, false)
	if err != nil {
		t.Fatalf("startDedicatedTor: %v", err)
	}
	if svc.OnionAddress() != "abcdefg.onion" {
		t.Errorf("got onion %q", svc.OnionAddress())
	}
	if svc.BackendPort() == 0 {
		t.Errorf("expected a nonzero backend port")
	}
	if err := svc.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck: %v", err)
	}
	client := svc.GetHTTPClient(false)
	if client == nil {
		t.Error("expected a non-nil direct http client")
	}
	if got := svc.GetHTTPClient(false).Timeout; got != 30*time.Second {
		t.Errorf("direct client timeout = %v, want 30s", got)
	}
	if err := svc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if !fake.closed {
		t.Error("expected underlying process to be closed")
	}
}

func TestStartDedicatedTorBootstrapFailure(t *testing.T) {
	withFakeBinaryFS(t, map[string]bool{"/usr/bin/tor": true}, "")
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseTorConfig()
	cfg.BootstrapTimeout = 1

	fake := &fakeTorProcess{enableErr: errors.New("bootstrap timed out")}
	origStart := startTorProcess
	startTorProcess = func(context.Context, *tor.StartConf) (torProcess, error) { return fake, nil }
	t.Cleanup(func() { startTorProcess = origStart })

	_, err := startDedicatedTor(context.Background(), cfg, p, nil, false)
	if err == nil {
		t.Fatal("expected bootstrap failure to propagate")
	}
	if !fake.closed {
		t.Error("expected process to be closed after bootstrap failure")
	}
}

func TestTorManagerMonitorRestartsUnhealthyProcess(t *testing.T) {
	withFakeBinaryFS(t, map[string]bool{"/usr/bin/tor": true}, "")
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseTorConfig()

	hsDir := filepath.Join(p.Data, "tor", "site")
	if err := os.MkdirAll(hsDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hsDir, "hostname"), []byte("abcdefg.onion\n"), 0o600); err != nil {
		t.Fatalf("write hostname: %v", err)
	}

	first := &fakeTorProcess{healthErr: errors.New("control conn dead")}
	second := &fakeTorProcess{}
	calls := 0
	origStart := startTorProcess
	startTorProcess = func(context.Context, *tor.StartConf) (torProcess, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}
	t.Cleanup(func() { startTorProcess = origStart })

	tm := NewTorManager(context.Background(), cfg, p, nil)
	if err := tm.startLocked(); err != nil {
		t.Fatalf("startLocked: %v", err)
	}

	tm.checkHealth()

	if !first.closed {
		t.Error("expected the unhealthy process to be closed")
	}
	if calls != 2 {
		t.Errorf("expected tor to be restarted once (2 starts total), got %d", calls)
	}
	if tm.Service() == nil {
		t.Fatal("expected a running service after restart")
	}

	if err := tm.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if !second.closed {
		t.Error("expected the replacement process to be closed on manager Close")
	}
}

func TestTorServiceDirectClientTypeIsHTTPClient(t *testing.T) {
	var svc TorService
	var c *http.Client = svc.GetHTTPClient(true)
	if c == nil {
		t.Fatal("expected a non-nil client even with no dialer configured")
	}
}
