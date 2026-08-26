package urlvars

import (
	"sort"
	"strings"
	"time"
)

// observation is a single host seen in a trusted reverse-proxy header.
type observation struct {
	host string
	at   time.Time
}

// observe records the resolved FQDN for the domain-learning algorithm
// and reports the FQDN the caller should use. When live reload is
// disabled the first resolved value is pinned and returned forever.
//
// Only hosts learned from a trusted reverse-proxy header feed the
// wildcard inference; an explicit multi-entry DOMAIN list disables
// learning entirely, per AI.md PART 8 ("Skip learning: If DOMAIN set").
func (r *Resolver) observe(fqdn string, fromProxy bool) string {
	r.mu.Lock()

	if !r.liveReload && r.pinned != "" {
		pinned := r.pinned
		r.mu.Unlock()
		return pinned
	}
	if r.pinned == "" {
		r.pinned = fqdn
	}

	if r.learning && fromProxy && len(r.domains) < 2 {
		now := r.now()
		r.pruneLocked(now)
		r.samples = append(r.samples, observation{host: fqdn, at: now})
	}

	previous := r.lastFQDN
	changed := previous != fqdn
	if changed {
		r.lastFQDN = fqdn
	}
	logChanges, logf, onChange := r.logChanges, r.logf, r.onChange

	r.mu.Unlock()

	if changed {
		if logChanges && logf != nil {
			logf("url detection: fqdn changed from %q to %q", previous, fqdn)
		}
		if onChange != nil {
			onChange(previous, fqdn)
		}
	}
	return fqdn
}

// pruneLocked drops observations that fell out of the sample window.
// The caller must hold the write lock.
func (r *Resolver) pruneLocked(now time.Time) {
	cutoff := now.Add(-r.sampleWindow)
	kept := r.samples[:0]
	for _, s := range r.samples {
		if s.at.After(cutoff) {
			kept = append(kept, s)
		}
	}
	r.samples = kept
}

// BaseDomain returns the inferred base domain: "myapp.com" even when the
// request arrived as "www.myapp.com". The first configured DOMAIN entry
// wins; otherwise the value is learned from observed hosts.
func (r *Resolver) BaseDomain() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.domains) > 0 {
		if base := BaseDomainOf(r.domains[0]); base != "" {
			return base
		}
		return r.domains[0]
	}
	if base, _ := r.dominantBaseLocked(); base != "" {
		return base
	}
	return BaseDomainOf(r.lastFQDN)
}

// WildcardDomain returns the inferred wildcard ("*.myapp.com") or "" when
// no wildcard pattern has been established. A multi-entry DOMAIN list
// establishes one directly as soon as two of its entries share a base
// domain; otherwise the pattern is learned, and needs min_samples
// distinct hosts within the sample window.
func (r *Resolver) WildcardDomain() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.domains) >= 2 {
		if base, distinct := dominantBase(r.domains); base != "" && distinct >= 2 {
			return "*." + base
		}
		return ""
	}

	base, distinct := r.dominantBaseLocked()
	if base == "" || distinct < r.minSamples {
		return ""
	}
	return "*." + base
}

// dominantBaseLocked returns the most-observed base domain within the
// sample window and how many distinct hosts contributed to it. The
// caller must hold at least the read lock.
func (r *Resolver) dominantBaseLocked() (string, int) {
	cutoff := r.now().Add(-r.sampleWindow)
	hosts := make([]string, 0, len(r.samples))
	for _, s := range r.samples {
		if s.at.After(cutoff) {
			hosts = append(hosts, s.host)
		}
	}
	return dominantBase(hosts)
}

// dominantBase groups hosts by their base domain and returns the base
// with the most distinct hosts, ties broken by total observations and
// then alphabetically so the result is deterministic.
func dominantBase(hosts []string) (string, int) {
	distinct := map[string]map[string]bool{}
	total := map[string]int{}

	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		base := BaseDomainOf(host)
		if base == "" {
			continue
		}
		if distinct[base] == nil {
			distinct[base] = map[string]bool{}
		}
		distinct[base][host] = true
		total[base]++
	}
	if len(distinct) == 0 {
		return "", 0
	}

	bases := make([]string, 0, len(distinct))
	for base := range distinct {
		bases = append(bases, base)
	}
	sort.Slice(bases, func(i, j int) bool {
		a, b := bases[i], bases[j]
		if len(distinct[a]) != len(distinct[b]) {
			return len(distinct[a]) > len(distinct[b])
		}
		if total[a] != total[b] {
			return total[a] > total[b]
		}
		return a < b
	})

	winner := bases[0]
	return winner, len(distinct[winner])
}
