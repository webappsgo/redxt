package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig is the subset of config.SMTPConfig this package needs to
// connect and send. It is a plain struct, not an import of the config
// package, so notify has no dependency on it and stays independently
// testable.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// TLS is one of "auto", "starttls", "tls", "none".
	TLS string
}

// candidatePorts are the ports AI.md PART 18's SMTP Auto-Detection
// table tries for every candidate host, in order.
var candidatePorts = []int{25, 465, 587}

// prober is the network probe SMTP autodetection and TestConnection
// use. It is a package variable so tests can substitute a fake that
// never touches the network or the clock.
type prober func(host string, port int, timeout time.Duration) error

// defaultProber attempts a real TCP connection and SMTP EHLO
// handshake against host:port, per AI.md PART 18 "Auto-Detection
// Process": step 2, "Attempt SMTP handshake (EHLO)".
func defaultProber(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.Hello("localhost")
}

// probe is the prober actually used at runtime; tests replace it.
var probe prober = defaultProber

// candidateHost is one host this package tried while autodetecting.
type candidateHost struct {
	host   string
	source string
}

// candidateHosts returns the AI.md PART 18 priority-ordered list of
// hosts to probe: loopback, the Docker bridge gateway, the machine's
// default gateway, the detected FQDN, the machine's global IPv4, and
// the mail./smtp. subdomains of the FQDN. Any source that cannot be
// determined (e.g. gatewayIP on a non-Linux host) is skipped, not
// substituted with a placeholder.
func candidateHosts(fqdn string) []candidateHost {
	var hosts []candidateHost

	add := func(host, source string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		hosts = append(hosts, candidateHost{host: host, source: source})
	}

	add("127.0.0.1", "loopback")
	add("172.17.0.1", "docker-bridge")
	add(gatewayIP(), "gateway")
	add(fqdn, "fqdn")
	add(globalIPv4(), "global-ipv4")
	if fqdn != "" {
		add("mail."+fqdn, "mail-subdomain")
		add("smtp."+fqdn, "smtp-subdomain")
	}

	return hosts
}

// AutodetectResult reports what Autodetect found.
type AutodetectResult struct {
	Host   string
	Port   int
	Source string
}

// Autodetect implements AI.md PART 18's "Auto-Detection Process": try
// every candidate host on ports 25, 465, and 587 in priority order,
// and return the first one whose SMTP handshake succeeds. It reports
// ok=false, not an error, when nothing is found — per the spec, "not
// an error, just no SMTP available".
func Autodetect(fqdn string, timeout time.Duration) (AutodetectResult, bool) {
	for _, c := range candidateHosts(fqdn) {
		for _, port := range candidatePorts {
			if probe(c.host, port, timeout) == nil {
				return AutodetectResult{Host: c.host, Port: port, Source: c.source}, true
			}
		}
	}
	return AutodetectResult{}, false
}

// TestConnection attempts a single SMTP handshake against host:port,
// used both by startup's "test the configured connection" behavior
// and the admin panel's "Send Test" / config-save validation button.
func TestConnection(host string, port int, timeout time.Duration) error {
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("notify: smtp host is empty")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("notify: smtp port %d is out of range", port)
	}
	return probe(host, port, timeout)
}

// gatewayIP makes a best-effort attempt to read the default IPv4
// gateway from /proc/net/route, per AI.md PART 18 priority 3. It
// returns "" on any non-Linux system, or if the route table has no
// default (destination 00000000) entry.
func gatewayIP() string {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	return parseGatewayIP(string(b))
}

// parseGatewayIP parses the contents of /proc/net/route and returns
// the gateway field of the default-route line (Destination 00000000),
// converted from its little-endian hex form to dotted-quad. It is
// split out from gatewayIP so it can be unit tested with fixed input
// instead of the real filesystem.
func parseGatewayIP(routeTable string) string {
	lines := strings.Split(routeTable, "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		ip := hexLEToIPv4(fields[2])
		if ip != "" {
			return ip
		}
	}
	return ""
}

// hexLEToIPv4 converts an 8-hex-digit little-endian value (the format
// /proc/net/route uses) into a dotted-quad IPv4 string.
func hexLEToIPv4(hexLE string) string {
	if len(hexLE) != 8 {
		return ""
	}
	var b [4]byte
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseUint(hexLE[i*2:i*2+2], 16, 8)
		if err != nil {
			return ""
		}
		b[3-i] = byte(v)
	}
	return net.IPv4(b[0], b[1], b[2], b[3]).String()
}

// globalIPv4 returns the machine's first non-loopback, non-RFC1918
// IPv4 address, per AI.md PART 18 priority 5. It returns "" if none
// is found.
func globalIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil || ip4.IsLoopback() || isPrivateIPv4(ip4) {
			continue
		}
		return ip4.String()
	}
	return ""
}

// isPrivateIPv4 reports whether ip falls in one of the RFC 1918
// private ranges.
func isPrivateIPv4(ip net.IP) bool {
	private := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	for _, cidr := range private {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil && block.Contains(ip) {
			return true
		}
	}
	return false
}

// Send delivers one email via stdlib net/smtp, honoring cfg.TLS per
// AI.md PART 18's "auto, starttls, tls, none" modes:
//   - "none": plain, unencrypted SMTP
//   - "starttls": plain connection upgraded with STARTTLS
//   - "tls": implicit TLS from the first byte (typically port 465)
//   - "auto": tls on port 465, starttls otherwise
func Send(ctx context.Context, cfg SMTPConfig, from, to, subject, body string) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("notify: smtp host is empty")
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.TLS))
	if mode == "" || mode == "auto" {
		if cfg.Port == 465 {
			mode = "tls"
		} else {
			mode = "starttls"
		}
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	msg := buildMessage(from, to, subject, body)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	dialer := &net.Dialer{}
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		dialer.Deadline = deadline
	}

	switch mode {
	case "tls":
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: cfg.Host})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return err
		}
		defer client.Close()
		return sendOverClient(client, auth, from, to, msg)

	case "starttls", "none":
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return err
		}
		defer client.Close()
		if mode == "starttls" {
			if ok, _ := client.Extension("STARTTLS"); ok {
				if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
					return err
				}
			}
		}
		return sendOverClient(client, auth, from, to, msg)

	default:
		return fmt.Errorf("notify: unknown smtp tls mode %q", cfg.TLS)
	}
}

// sendOverClient runs the MAIL/RCPT/DATA sequence on an already
// connected, already TLS-negotiated client.
func sendOverClient(client *smtp.Client, auth smtp.Auth, from, to, msg string) error {
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// buildMessage renders the RFC 5322 headers and body for one email.
func buildMessage(from, to, subject, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return b.String()
}
