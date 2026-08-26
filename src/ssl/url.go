package ssl

import "fmt"

// FormatURL builds a display URL for host on port, per the PART 15 URL
// format rules. Port 443 always implies https, and both :80 and :443 are
// always stripped from the result; every other port is written out.
//
// The host is used verbatim, so an IPv6 literal must already be bracketed.
func FormatURL(host string, port int, isHTTPS bool) string {
	proto := "http"
	if isHTTPS || port == 443 {
		proto = "https"
	}

	// Always strip the default ports for their scheme.
	if port == 80 || port == 443 {
		return proto + "://" + host
	}

	return fmt.Sprintf("%s://%s:%d", proto, host, port)
}
