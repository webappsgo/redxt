package urlvars

import "net"

// IsPublicIP reports whether ip is a publicly routable unicast address.
// It excludes loopback, RFC 1918 (10/8, 172.16/12, 192.168/16), IPv4
// link-local (169.254/16), IPv6 loopback (::1), IPv6 link-local
// (fe80::/10) and IPv6 unique-local (fc00::/7), matching the ranges
// listed as always trusted in AI.md PART 12.
func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	if ip.IsPrivate() || ip.IsLoopback() {
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}

// SelectPublicIPs returns the first public IPv4 and the first public
// IPv6 from ips, in the order given. Either result is empty when no
// address of that family qualifies.
func SelectPublicIPs(ips []net.IP) (ipv4, ipv6 string) {
	for _, ip := range ips {
		if !IsPublicIP(ip) {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			if ipv4 == "" {
				ipv4 = v4.String()
			}
			continue
		}
		if ipv6 == "" {
			ipv6 = ip.String()
		}
	}
	return ipv4, ipv6
}

// PublicIPs enumerates the machine's interfaces and returns its first
// public IPv4 and IPv6 addresses. Both results are empty when the host
// has no publicly routable address.
func PublicIPs() (ipv4, ipv6 string) {
	return SelectPublicIPs(interfaceIPs())
}

// interfaceIPs collects the unicast addresses of every interface that
// is up and not a loopback.
func interfaceIPs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	out := []net.IP{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				out = append(out, v.IP)
			case *net.IPAddr:
				out = append(out, v.IP)
			}
		}
	}
	return out
}
