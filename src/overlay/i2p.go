package overlay

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/paths"
)

// i2pdCommonPaths are the well-known i2pd install locations checked when
// cfg.Binary is empty, before falling back to $PATH. Order matches AI.md
// PART 32.2 "I2P Process Management".
var i2pdCommonPaths = []string{
	"/usr/bin/i2pd",
	"/usr/sbin/i2pd",
	"/usr/local/bin/i2pd",
	"/opt/homebrew/bin/i2pd",
}

// resolveI2PDBinary locates the i2pd executable: an explicit cfg.Binary
// override wins, then common install locations, then $PATH. Returns an
// error when no i2pd binary is available - in which case the caller falls
// back to SAM (Model B).
func resolveI2PDBinary(cfg *config.I2P) (string, error) {
	if cfg.Binary != "" {
		if _, err := statFile(cfg.Binary); err == nil {
			return cfg.Binary, nil
		}
		return "", fmt.Errorf("configured i2pd binary not found: %s", cfg.Binary)
	}
	for _, p := range i2pdCommonPaths {
		if _, err := statFile(p); err == nil {
			return p, nil
		}
	}
	if p, err := lookPath("i2pd"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("i2pd binary not found")
}

// dialSAM opens a TCP connection to a SAM bridge. It is a package var so
// tests can point it at a local net.Pipe/fake listener instead of a real
// I2P router.
var dialSAM = func(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

// samReachable reports whether a SAMv3 bridge is accepting connections at
// addr.
func samReachable(addr string) bool {
	conn, err := dialSAM(addr, 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ensureI2PDirs creates {config_dir}/i2p, {data_dir}/i2p, and
// {data_dir}/i2p/site with 0700 permissions, enforced idempotently on every
// call (mirrors ensureTorDirs).
func ensureI2PDirs(p paths.Paths) error {
	for _, dir := range []string{
		filepath.Join(p.Config, "i2p"),
		filepath.Join(p.Data, "i2p"),
		filepath.Join(p.Data, "i2p", "site"),
	} {
		if err := ensureDir(dir); err != nil {
			return err
		}
	}
	return nil
}

// updateI2PTunnels (over)writes tunnels.conf with the given content.
func updateI2PTunnels(path string, content []byte) error {
	return writeSecureFile(path, content)
}

// getI2PTunnelsConf generates the i2pd server-tunnel definition. The
// eepsite is declared here via a [site] server tunnel pointing at the
// dedicated backend port; i2pd persists the destination in keysPath and
// derives the .b32.i2p address from it.
func getI2PTunnelsConf(cfg *config.I2P, keysPath string, i2pBackendPort int) string {
	return fmt.Sprintf(`[site]
type = server
host = 127.0.0.1
port = %d
keys = %s
inbound.length = %d
outbound.length = %d
inbound.quantity = %d
outbound.quantity = %d
signaturetype = %d
`, i2pBackendPort, keysPath,
		cfg.InboundLength, cfg.OutboundLength,
		cfg.InboundQuantity, cfg.OutboundQuantity, cfg.SignatureType)
}

// I2PProvider identifies which backend created the eepsite.
type I2PProvider int

const (
	// I2PProviderNone means no provider was available (I2P disabled).
	I2PProviderNone I2PProvider = iota
	// I2PProviderI2PD spawns and manages a dedicated i2pd process (Model A).
	I2PProviderI2PD
	// I2PProviderSAM uses an external SAMv3 bridge (Model B).
	I2PProviderSAM
)

// String returns the provider name used in logs and CLI/admin status.
func (p I2PProvider) String() string {
	switch p {
	case I2PProviderI2PD:
		return "i2pd"
	case I2PProviderSAM:
		return "sam"
	default:
		return "none"
	}
}

// startI2PDProcess starts a real dedicated i2pd process. Overridden in
// tests so no test ever spawns a real i2pd binary or touches the I2P
// network.
var startI2PDProcess = func(ctx context.Context, binary string, args []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// i2pdAddressPollInterval is how often waitForI2PDAddress rechecks the
// destination keyfile while waiting for i2pd to publish it.
var i2pdAddressPollInterval = 500 * time.Millisecond

// I2PService manages the I2P eepsite. Server binary owns the provider
// lifecycle: the full i2pd process for Model A, or the SAM session for
// Model B.
type I2PService struct {
	provider       I2PProvider
	eepsiteAddress string
	i2pBackendPort int
	// Model A: the managed i2pd process (nil for Model B)
	i2pd *exec.Cmd
	// Model B: the live SAM control connection (nil for Model A)
	samConn net.Conn
}

// EepsiteAddress returns the full .b32.i2p address.
func (s *I2PService) EepsiteAddress() string { return s.eepsiteAddress }

// Provider returns which backend is serving the eepsite.
func (s *I2PService) Provider() I2PProvider { return s.provider }

// BackendPort returns the dedicated plain loopback port the eepsite
// forwards to. Pass it to NewI2PBackendListener.
func (s *I2PService) BackendPort() int { return s.i2pBackendPort }

// HealthCheck reports whether the provider is still alive. The caller
// registers this as the i2p_health scheduler task; this package never
// imports src/scheduler or registers anything itself.
func (s *I2PService) HealthCheck(context.Context) error {
	switch s.provider {
	case I2PProviderI2PD:
		if s.i2pd == nil || s.i2pd.Process == nil {
			return fmt.Errorf("i2pd process not running")
		}
		if s.i2pd.ProcessState != nil {
			return fmt.Errorf("i2pd process exited")
		}
		return nil
	case I2PProviderSAM:
		if s.samConn == nil {
			return fmt.Errorf("sam session not connected")
		}
		return nil
	default:
		return fmt.Errorf("i2p not running")
	}
}

// Close shuts down the provider (i2pd process or SAM session).
func (s *I2PService) Close() error {
	if s.samConn != nil {
		err := s.samConn.Close()
		s.samConn = nil
		return err
	}
	if s.i2pd != nil && s.i2pd.Process != nil {
		err := s.i2pd.Process.Signal(os.Interrupt)
		s.i2pd = nil
		return err
	}
	return nil
}

// NewI2PBackendListener creates the dedicated plain loopback listener the
// eepsite forwards to. Unlike Tor, neither i2pd nor SAM prepends a
// PROXY-protocol header, so this listener is NOT wrapped with
// go-proxyproto. It is a separate listener from the clearnet HTTP port.
// Pass svc.BackendPort() as port.
func NewI2PBackendListener(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen i2p backend port %d: %w", port, err)
	}
	return ln, nil
}

// startDedicatedI2P creates the eepsite when I2P is enabled AND a provider
// is available. It resolves the provider FIRST (i2pd binary, else a
// reachable SAM bridge); if neither is available it returns an error and NO
// backend port is allocated (I2P stays disabled). Nothing in this function,
// or anywhere in this package, ever sets cfg.Enabled.
func startDedicatedI2P(ctx context.Context, cfg *config.I2P, p paths.Paths, log Logger) (*I2PService, error) {
	log = orDiscard(log)

	if !cfg.Enabled {
		return nil, fmt.Errorf("i2p disabled (opt-in) - eepsite not started")
	}

	provider := I2PProviderNone
	i2pdBinary := ""
	if b, err := resolveI2PDBinary(cfg); err == nil {
		provider, i2pdBinary = I2PProviderI2PD, b
	} else if samReachable(cfg.SAMAddress) {
		provider = I2PProviderSAM
	} else {
		log.Warnf("i2p enabled but no provider available (no i2pd binary, SAM %s unreachable)", cfg.SAMAddress)
		return nil, fmt.Errorf("i2p enabled but no provider available (no i2pd binary, SAM %s unreachable)", cfg.SAMAddress)
	}

	if err := ensureI2PDirs(p); err != nil {
		return nil, fmt.Errorf("failed to create i2p directories: %w", err)
	}

	// Allocate the dedicated plain loopback port only now that a provider is
	// confirmed. Not persisted: a fresh port each run.
	i2pBackendPort := config.RandomUnusedPort()

	// The destination key persists here; the .b32.i2p address derives from
	// it.
	keysPath := filepath.Join(p.Data, "i2p", "site", "site-keys.dat")

	svc := &I2PService{provider: provider, i2pBackendPort: i2pBackendPort}

	var addr string
	var err error
	switch provider {
	case I2PProviderI2PD:
		addr, err = startI2PD(ctx, cfg, i2pdBinary, keysPath, i2pBackendPort, svc, p)
	case I2PProviderSAM:
		addr, err = startSAMEepsite(ctx, cfg, keysPath, i2pBackendPort, svc)
	}
	if err != nil {
		return nil, err
	}
	svc.eepsiteAddress = addr

	log.Infof("I2P eepsite started (%s): %s:%d -> 127.0.0.1:%d",
		provider.String(), svc.eepsiteAddress, cfg.VirtualPort, i2pBackendPort)
	return svc, nil
}

// startI2PD writes tunnels.conf (regenerated each run) and starts a
// dedicated i2pd child process, then waits for it to publish the
// destination and derives the .b32.i2p from it. i2pd creates/persists
// site-keys.dat at keysPath.
func startI2PD(ctx context.Context, cfg *config.I2P, binary, keysPath string, i2pBackendPort int, svc *I2PService, p paths.Paths) (string, error) {
	tunnelsPath := filepath.Join(p.Config, "i2p", "tunnels.conf")

	conf := getI2PTunnelsConf(cfg, keysPath, i2pBackendPort)
	if err := updateI2PTunnels(tunnelsPath, []byte(conf)); err != nil {
		return "", fmt.Errorf("failed to write tunnels.conf: %w", err)
	}

	cmd, err := startI2PDProcess(ctx, binary, []string{
		"--datadir", filepath.Join(p.Data, "i2p"),
		"--tunconf", tunnelsPath,
		"--log", "file",
		"--logfile", filepath.Join(p.Logs, "i2pd.log"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to start i2pd: %w", err)
	}
	svc.i2pd = cmd

	deadline := time.Duration(cfg.BootstrapTimeout) * time.Second
	addr, err := waitForI2PDAddress(ctx, keysPath, deadline)
	if err != nil {
		cmd.Process.Signal(os.Interrupt)
		return "", err
	}
	return addr, nil
}

// destinationLength reports the length in bytes of the I2P Destination
// (KeysAndCert) prefix at the start of data: a 256-byte public key, a
// 128-byte signing-key slot, and a Certificate (1-byte type, 2-byte
// big-endian length, then that many bytes of payload). This layout is fixed
// by the I2P common-structures spec regardless of signature type, so it
// applies to both the raw destination i2pd derives its .b32.i2p from and
// the private-key file i2pd persists (which starts with the destination).
func destinationLength(data []byte) (int, bool) {
	const fixedPrefix = 384
	if len(data) < fixedPrefix+3 {
		return 0, false
	}
	certLen := int(data[fixedPrefix+1])<<8 | int(data[fixedPrefix+2])
	total := fixedPrefix + 3 + certLen
	if len(data) < total {
		return 0, false
	}
	return total, true
}

// b32Address derives the .b32.i2p address: base32(sha256(destination))
// without padding, lowercased, plus the ".b32.i2p" suffix.
func b32Address(destBinary []byte) string {
	sum := sha256.Sum256(destBinary)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(sum[:])) + ".b32.i2p"
}

// waitForI2PDAddress polls keysPath until i2pd has written a destination
// long enough to contain a full Destination structure, then derives the
// .b32.i2p address from it.
func waitForI2PDAddress(ctx context.Context, keysPath string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(keysPath); err == nil {
			if destLen, ok := destinationLength(data); ok {
				return b32Address(data[:destLen]), nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for i2pd destination at %s", keysPath)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(i2pdAddressPollInterval):
		}
	}
}

// i2pBase64 is the SAMv3/I2P base64 alphabet: standard base64 with "-" in
// place of "+" and "~" in place of "/", unpadded.
var i2pBase64 = base64.NewEncoding(
	"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-~",
).WithPadding(base64.NoPadding)

// decodeI2PBase64 decodes a SAM-alphabet base64 string, tolerating a
// trailing "=" some routers still emit despite the format being unpadded.
func decodeI2PBase64(s string) ([]byte, error) {
	return i2pBase64.DecodeString(strings.TrimRight(s, "="))
}

// readSAMReply reads one line from r and requires it to start with
// wantPrefix and carry RESULT=OK.
func readSAMReply(r *bufio.Reader, wantPrefix string) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read SAM reply: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, wantPrefix) {
		return "", fmt.Errorf("unexpected SAM reply: %s", line)
	}
	if !strings.Contains(line, "RESULT=OK") {
		return "", fmt.Errorf("SAM error: %s", line)
	}
	return line, nil
}

// samField extracts the value of KEY=value out of a SAM reply line.
func samField(line, key string) (string, bool) {
	prefix := key + "="
	for _, tok := range strings.Fields(line) {
		if strings.HasPrefix(tok, prefix) {
			return strings.TrimPrefix(tok, prefix), true
		}
	}
	return "", false
}

// samDestination holds a SAM destination's private key string (as SAM's
// DESTINATION= parameter expects it) and its decoded public bytes (as
// b32Address expects them).
type samDestination struct {
	priv string
	pub  []byte
}

// parsePersistedSAMDestination decodes a "PRIV=...\nPUB=...\n" file as
// written by writePersistedSAMDestination.
func parsePersistedSAMDestination(data []byte) (*samDestination, bool) {
	priv, pub := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "PRIV="):
			priv = strings.TrimPrefix(line, "PRIV=")
		case strings.HasPrefix(line, "PUB="):
			pub = strings.TrimPrefix(line, "PUB=")
		}
	}
	if priv == "" || pub == "" {
		return nil, false
	}
	pubBytes, err := decodeI2PBase64(pub)
	if err != nil {
		return nil, false
	}
	return &samDestination{priv: priv, pub: pubBytes}, true
}

// writePersistedSAMDestination persists dest to keysPath so subsequent
// restarts reuse the same .b32.i2p address instead of generating a new one.
func writePersistedSAMDestination(keysPath, priv, pubB64 string) error {
	content := fmt.Sprintf("PRIV=%s\nPUB=%s\n", priv, pubB64)
	return writeSecureFile(keysPath, []byte(content))
}

// loadOrCreateSAMDestination loads the persisted destination from keysPath,
// or - if none is persisted yet - issues DEST GENERATE over the SAM control
// connection and persists the result.
func loadOrCreateSAMDestination(conn net.Conn, r *bufio.Reader, keysPath string, sigType int) (*samDestination, error) {
	if data, err := os.ReadFile(keysPath); err == nil {
		if dest, ok := parsePersistedSAMDestination(data); ok {
			return dest, nil
		}
	}

	if _, err := fmt.Fprintf(conn, "DEST GENERATE SIGNATURE_TYPE=%d\n", sigType); err != nil {
		return nil, fmt.Errorf("send DEST GENERATE: %w", err)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read DEST GENERATE reply: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	pubB64, ok := samField(line, "PUB")
	if !ok {
		return nil, fmt.Errorf("DEST GENERATE reply missing PUB: %s", line)
	}
	priv, ok := samField(line, "PRIV")
	if !ok {
		return nil, fmt.Errorf("DEST GENERATE reply missing PRIV: %s", line)
	}
	pubBytes, err := decodeI2PBase64(pubB64)
	if err != nil {
		return nil, fmt.Errorf("decode destination public key: %w", err)
	}
	if err := writePersistedSAMDestination(keysPath, priv, pubB64); err != nil {
		return nil, fmt.Errorf("persist i2p destination: %w", err)
	}
	return &samDestination{priv: priv, pub: pubBytes}, nil
}

// startSAMEepsite opens a SAMv3 control connection, loads (or generates and
// persists) the destination, creates a STREAM session, and forwards
// incoming streams to the dedicated backend port. Returns the .b32.i2p
// address. The control connection is kept open for the session's lifetime.
func startSAMEepsite(ctx context.Context, cfg *config.I2P, keysPath string, i2pBackendPort int, svc *I2PService) (string, error) {
	conn, err := dialSAM(cfg.SAMAddress, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to dial SAM %s: %w", cfg.SAMAddress, err)
	}
	r := bufio.NewReader(conn)

	// 1. Handshake
	if _, err := fmt.Fprintf(conn, "HELLO VERSION MIN=3.0 MAX=3.3\n"); err != nil {
		conn.Close()
		return "", err
	}
	if _, err := readSAMReply(r, "HELLO REPLY"); err != nil {
		conn.Close()
		return "", err
	}

	// 2. Load persisted destination or generate + persist a new one.
	dest, err := loadOrCreateSAMDestination(conn, r, keysPath, cfg.SignatureType)
	if err != nil {
		conn.Close()
		return "", err
	}

	// 3. Create the STREAM session bound to that destination.
	if _, err := fmt.Fprintf(conn, "SESSION CREATE STYLE=STREAM ID=site DESTINATION=%s "+
		"inbound.length=%d outbound.length=%d inbound.quantity=%d outbound.quantity=%d\n",
		dest.priv, cfg.InboundLength, cfg.OutboundLength, cfg.InboundQuantity, cfg.OutboundQuantity); err != nil {
		conn.Close()
		return "", err
	}
	if _, err := readSAMReply(r, "SESSION STATUS"); err != nil {
		conn.Close()
		return "", err
	}

	// 4. Forward incoming eepsite streams to the backend port.
	if _, err := fmt.Fprintf(conn, "STREAM FORWARD ID=site PORT=%d HOST=127.0.0.1\n", i2pBackendPort); err != nil {
		conn.Close()
		return "", err
	}
	if _, err := readSAMReply(r, "STREAM STATUS"); err != nil {
		conn.Close()
		return "", err
	}

	svc.samConn = conn
	return b32Address(dest.pub), nil
}

// I2PManager owns the running I2PService and handles start/restart/
// regenerate operations. It stores NO backend port itself - the dedicated
// port is allocated inside startDedicatedI2P (only when I2P is enabled AND
// a provider is available).
type I2PManager struct {
	mu      sync.Mutex
	service *I2PService
	config  *config.I2P
	paths   paths.Paths
	log     Logger
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewI2PManager creates a new I2P manager with the given configuration.
func NewI2PManager(ctx context.Context, cfg *config.I2P, p paths.Paths, log Logger) *I2PManager {
	return &I2PManager{
		config: cfg,
		paths:  p,
		log:    orDiscard(log),
		ctx:    ctx,
	}
}

// Start initializes the eepsite if I2P is enabled and a provider is
// available, and starts the health monitor. A disabled I2P config is not an
// error: Start returns nil and no eepsite runs.
func (im *I2PManager) Start() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if !im.config.Enabled {
		return nil
	}
	if err := im.startLocked(); err != nil {
		return err
	}
	im.startMonitorLocked()
	return nil
}

func (im *I2PManager) startLocked() error {
	service, err := startDedicatedI2P(im.ctx, im.config, im.paths, im.log)
	if err != nil {
		return err
	}
	im.service = service
	return nil
}

func (im *I2PManager) startMonitorLocked() {
	monCtx, cancel := context.WithCancel(im.ctx)
	im.cancel = cancel
	im.wg.Add(1)
	go func() {
		defer im.wg.Done()
		monitorI2P(monCtx, im)
	}()
}

// UpdateConfig applies new settings and restarts I2P. If the new config
// disables I2P, the eepsite stays down - opt-in respected.
func (im *I2PManager) UpdateConfig(cfg *config.I2P) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.config = cfg
	if im.service != nil {
		im.service.Close()
		im.service = nil
	}
	if !cfg.Enabled {
		return nil
	}
	return im.startLocked()
}

// RegenerateAddress creates a new random .b32.i2p destination.
func (im *I2PManager) RegenerateAddress() (string, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		im.service.Close()
		im.service = nil
	}
	if err := os.RemoveAll(filepath.Join(im.paths.Data, "i2p", "site")); err != nil {
		return "", fmt.Errorf("failed to remove old i2p keys: %w", err)
	}
	if err := im.startLocked(); err != nil {
		return "", err
	}
	return im.service.EepsiteAddress(), nil
}

// EepsiteAddress returns the current .b32.i2p address (empty if not
// running).
func (im *I2PManager) EepsiteAddress() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return im.service.EepsiteAddress()
	}
	return ""
}

// Service returns the currently running I2PService, or nil.
func (im *I2PManager) Service() *I2PService {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.service
}

// Close stops the health monitor and the provider. This runs synchronously
// so the overlay process is fully stopped before a caller goes on to cancel
// the context it passed to NewI2PManager.
func (im *I2PManager) Close() error {
	im.mu.Lock()
	cancel := im.cancel
	im.cancel = nil
	im.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	im.wg.Wait()

	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		err := im.service.Close()
		im.service = nil
		return err
	}
	return nil
}

// monitorI2P is the 30s-ticker health/restart loop for the I2P provider
// (mirrors monitorTor).
func monitorI2P(ctx context.Context, im *I2PManager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			im.checkHealth()
		}
	}
}

func (im *I2PManager) checkHealth() {
	im.mu.Lock()
	svc := im.service
	im.mu.Unlock()
	if svc == nil {
		return
	}
	if err := svc.HealthCheck(context.Background()); err == nil {
		return
	}
	im.log.Warnf("I2P provider unresponsive, restarting...")
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		im.service.Close()
		im.service = nil
	}
	if err := im.startLocked(); err != nil {
		im.log.Warnf("Failed to restart I2P: %v", err)
	}
}
