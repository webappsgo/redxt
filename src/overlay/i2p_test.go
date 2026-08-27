package overlay

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/paths"
)

func TestResolveI2PDBinary(t *testing.T) {
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
			cfgBinary:  "/custom/i2pd",
			existing:   map[string]bool{"/custom/i2pd": true, "/usr/bin/i2pd": true},
			wantBinary: "/custom/i2pd",
		},
		{
			name:      "explicit config path missing is an error, no fallback",
			cfgBinary: "/custom/i2pd",
			existing:  map[string]bool{"/usr/bin/i2pd": true},
			wantErr:   true,
		},
		{
			name:       "well-known path found",
			existing:   map[string]bool{"/usr/sbin/i2pd": true},
			wantBinary: "/usr/sbin/i2pd",
		},
		{
			name:       "falls back to $PATH",
			existing:   map[string]bool{},
			pathBin:    "/opt/bin/i2pd",
			wantBinary: "/opt/bin/i2pd",
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
			got, err := resolveI2PDBinary(&config.I2P{Binary: tc.cfgBinary})
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

func baseI2PConfig() *config.I2P {
	return &config.I2P{
		Enabled:          true,
		SAMAddress:       "127.0.0.1:7656",
		VirtualPort:      80,
		InboundLength:    3,
		OutboundLength:   3,
		InboundQuantity:  5,
		OutboundQuantity: 5,
		SignatureType:    7,
		BootstrapTimeout: 30,
	}
}

func TestGetI2PTunnelsConf(t *testing.T) {
	cfg := baseI2PConfig()
	got := getI2PTunnelsConf(cfg, "/data/i2p/site/site-keys.dat", 54321)

	want := `[site]
type = server
host = 127.0.0.1
port = 54321
keys = /data/i2p/site/site-keys.dat
inbound.length = 3
outbound.length = 3
inbound.quantity = 5
outbound.quantity = 5
signaturetype = 7
`
	if got != want {
		t.Errorf("tunnels.conf mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestB32AddressKnownAnswer(t *testing.T) {
	dest := []byte("this is a stand-in for a real 387+ byte I2P destination structure")
	sum := sha256.Sum256(dest)
	want := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])) + ".b32.i2p"

	got := b32Address(dest)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, ".b32.i2p") {
		t.Errorf("missing .b32.i2p suffix: %q", got)
	}
	if strings.ToLower(got) != got {
		t.Errorf("expected lowercase address, got %q", got)
	}
	if strings.Contains(got, "=") {
		t.Errorf("expected unpadded base32, got %q", got)
	}
}

func TestEnsureI2PDirsPermissions(t *testing.T) {
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}

	if err := ensureI2PDirs(p); err != nil {
		t.Fatalf("ensureI2PDirs: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(p.Config, "i2p"),
		filepath.Join(p.Data, "i2p"),
		filepath.Join(p.Data, "i2p", "site"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("dir %s has perm %o, want 0700", dir, perm)
		}
	}
}

// pipeConn wraps one end of a net.Pipe so it satisfies net.Conn with the
// deadline methods dialSAM callers rely on (net.Pipe's Conn already does).
func fakeSAMServer(t *testing.T, script func(r *bufio.Reader, w net.Conn)) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		script(bufio.NewReader(conn), conn)
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestSAMHandshakeAndEepsiteStart(t *testing.T) {
	tmp := t.TempDir()
	keysPath := filepath.Join(tmp, "site-keys.dat")

	// A realistic-length fake public destination so decodeI2PBase64 succeeds.
	fakePub := strings.Repeat("A", 516)
	fakePriv := strings.Repeat("A", 663)

	addr, cleanup := fakeSAMServer(t, func(r *bufio.Reader, w net.Conn) {
		line, _ := r.ReadString('\n')
		if !strings.HasPrefix(line, "HELLO VERSION") {
			t.Errorf("unexpected first line: %q", line)
		}
		w.Write([]byte("HELLO REPLY RESULT=OK VERSION=3.1\n"))

		line, _ = r.ReadString('\n')
		if !strings.HasPrefix(line, "DEST GENERATE") {
			t.Errorf("expected DEST GENERATE, got %q", line)
		}
		w.Write([]byte("DEST REPLY PUB=" + fakePub + " PRIV=" + fakePriv + "\n"))

		line, _ = r.ReadString('\n')
		if !strings.HasPrefix(line, "SESSION CREATE") {
			t.Errorf("expected SESSION CREATE, got %q", line)
		}
		w.Write([]byte("SESSION STATUS RESULT=OK\n"))

		line, _ = r.ReadString('\n')
		if !strings.HasPrefix(line, "STREAM FORWARD") {
			t.Errorf("expected STREAM FORWARD, got %q", line)
		}
		w.Write([]byte("STREAM STATUS RESULT=OK\n"))
	})
	defer cleanup()

	origDial := dialSAM
	dialSAM = func(a string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("tcp", addr, timeout)
	}
	t.Cleanup(func() { dialSAM = origDial })

	cfg := baseI2PConfig()
	cfg.SAMAddress = addr
	svc := &I2PService{}

	got, err := startSAMEepsite(context.Background(), cfg, keysPath, 54321, svc)
	if err != nil {
		t.Fatalf("startSAMEepsite: %v", err)
	}
	if !strings.HasSuffix(got, ".b32.i2p") {
		t.Errorf("expected .b32.i2p address, got %q", got)
	}
	if svc.samConn == nil {
		t.Error("expected samConn to be kept open")
	}

	if _, err := os.Stat(keysPath); err != nil {
		t.Errorf("expected destination to be persisted: %v", err)
	}

	svc.Close()
}

func TestLoadOrCreateSAMDestinationReusesPersisted(t *testing.T) {
	tmp := t.TempDir()
	keysPath := filepath.Join(tmp, "site-keys.dat")

	pubB64 := strings.Repeat("B", 516)
	priv := strings.Repeat("B", 663)
	if err := writePersistedSAMDestination(keysPath, priv, pubB64); err != nil {
		t.Fatalf("writePersistedSAMDestination: %v", err)
	}

	dest, err := loadOrCreateSAMDestination(nil, nil, keysPath, 7)
	if err != nil {
		t.Fatalf("loadOrCreateSAMDestination: %v", err)
	}
	if dest.priv != priv {
		t.Errorf("priv mismatch: got %q", dest.priv)
	}
	if len(dest.pub) == 0 {
		t.Error("expected decoded pub bytes")
	}
}

func TestDecodeI2PBase64Alphabet(t *testing.T) {
	// "-" and "~" are the SAM-alphabet substitutes for "+" and "/".
	encoded := "AA-A~A"
	if _, err := decodeI2PBase64(encoded); err != nil {
		t.Errorf("expected SAM-alphabet string to decode: %v", err)
	}
}

func TestDestinationLength(t *testing.T) {
	payload := make([]byte, 384)
	// cert type 0, cert length 0 -> total length 384+3+0
	payload = append(payload, 0x00, 0x00, 0x00)
	n, ok := destinationLength(payload)
	if !ok {
		t.Fatal("expected destinationLength to succeed")
	}
	if n != 387 {
		t.Errorf("got %d, want 387", n)
	}

	short := make([]byte, 10)
	if _, ok := destinationLength(short); ok {
		t.Error("expected failure on too-short input")
	}
}

func TestStartDedicatedI2PDisabled(t *testing.T) {
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseI2PConfig()
	cfg.Enabled = false

	_, err := startDedicatedI2P(context.Background(), cfg, p, nil)
	if err == nil {
		t.Fatal("expected error when I2P is disabled")
	}
}

func TestStartDedicatedI2PNoProvider(t *testing.T) {
	withFakeBinaryFS(t, map[string]bool{}, "")
	origDial := dialSAM
	dialSAM = func(string, time.Duration) (net.Conn, error) { return nil, errors.New("unreachable") }
	t.Cleanup(func() { dialSAM = origDial })

	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseI2PConfig()

	_, err := startDedicatedI2P(context.Background(), cfg, p, nil)
	if err == nil {
		t.Fatal("expected error when neither i2pd nor SAM is available")
	}
}

func TestStartDedicatedI2PModelA(t *testing.T) {
	withFakeBinaryFS(t, map[string]bool{"/usr/bin/i2pd": true}, "")

	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseI2PConfig()
	cfg.BootstrapTimeout = 2

	origPoll := i2pdAddressPollInterval
	i2pdAddressPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { i2pdAddressPollInterval = origPoll })

	origStart := startI2PDProcess
	startI2PDProcess = func(ctx context.Context, binary string, args []string) (*exec.Cmd, error) {
		if binary != "/usr/bin/i2pd" {
			t.Errorf("unexpected binary %q", binary)
		}
		// Simulate i2pd publishing the destination shortly after start.
		keysPath := filepath.Join(p.Data, "i2p", "site", "site-keys.dat")
		go func() {
			time.Sleep(20 * time.Millisecond)
			payload := make([]byte, 384)
			payload = append(payload, 0x00, 0x00, 0x00)
			os.WriteFile(keysPath, payload, 0o600)
		}()
		return exec.Command("true"), nil
	}
	t.Cleanup(func() { startI2PDProcess = origStart })

	svc, err := startDedicatedI2P(context.Background(), cfg, p, nil)
	if err != nil {
		t.Fatalf("startDedicatedI2P: %v", err)
	}
	if svc.Provider() != I2PProviderI2PD {
		t.Errorf("expected I2PProviderI2PD, got %v", svc.Provider())
	}
	if !strings.HasSuffix(svc.EepsiteAddress(), ".b32.i2p") {
		t.Errorf("expected .b32.i2p address, got %q", svc.EepsiteAddress())
	}

	tunnelsPath := filepath.Join(p.Config, "i2p", "tunnels.conf")
	if _, err := os.Stat(tunnelsPath); err != nil {
		t.Errorf("expected tunnels.conf to be written: %v", err)
	}
}

func TestNewI2PBackendListenerIsPlain(t *testing.T) {
	ln, err := NewI2PBackendListener(0)
	if err != nil {
		t.Fatalf("NewI2PBackendListener: %v", err)
	}
	defer ln.Close()

	// A plain net.Listener from net.Listen, not wrapped with proxyproto.
	if _, ok := ln.(*net.TCPListener); !ok {
		t.Errorf("expected *net.TCPListener, got %T", ln)
	}
}

func TestI2PManagerUpdateConfigDisablingStopsService(t *testing.T) {
	withFakeBinaryFS(t, map[string]bool{"/usr/bin/i2pd": true}, "")
	tmp := t.TempDir()
	p := paths.Paths{Config: filepath.Join(tmp, "config"), Data: filepath.Join(tmp, "data")}
	cfg := baseI2PConfig()
	cfg.BootstrapTimeout = 2

	origPoll := i2pdAddressPollInterval
	i2pdAddressPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { i2pdAddressPollInterval = origPoll })

	origStart := startI2PDProcess
	startI2PDProcess = func(ctx context.Context, binary string, args []string) (*exec.Cmd, error) {
		keysPath := filepath.Join(p.Data, "i2p", "site", "site-keys.dat")
		go func() {
			time.Sleep(10 * time.Millisecond)
			payload := make([]byte, 384)
			payload = append(payload, 0x00, 0x00, 0x00)
			os.WriteFile(keysPath, payload, 0o600)
		}()
		return exec.Command("true"), nil
	}
	t.Cleanup(func() { startI2PDProcess = origStart })

	im := NewI2PManager(context.Background(), cfg, p, nil)
	if err := im.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if im.Service() == nil {
		t.Fatal("expected a running service")
	}

	disabled := baseI2PConfig()
	disabled.Enabled = false
	if err := im.UpdateConfig(disabled); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if im.Service() != nil {
		t.Error("expected service to be nil after disabling I2P")
	}

	if err := im.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
