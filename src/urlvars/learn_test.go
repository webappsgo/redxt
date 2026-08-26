package urlvars

import (
	"testing"
	"time"
)

// learningResolver builds a resolver with learning enabled, an injected
// clock, and no DOMAIN so every value comes from observed requests.
func learningResolver(t *testing.T, clock *time.Time, mutate func(*Options)) *Resolver {
	t.Helper()

	opts := baseOptions()
	opts.Domain = ""
	opts.Learning = LearningOptions{
		Enabled:      Bool(true),
		MinSamples:   3,
		SampleWindow: time.Minute,
	}
	opts.Now = func() time.Time { return *clock }
	if mutate != nil {
		mutate(&opts)
	}
	return New(opts)
}

// visit resolves one request whose trusted proxy reports host.
func visit(r *Resolver, host, remote string) string {
	_, fqdn, _ := r.URLVars(newRequest("internal", remote, map[string]string{HeaderForwardedHost: host}))
	return fqdn
}

func TestLearningInfersWildcard(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, nil)

	visit(r, "myapp.com", trustedPeer)
	visit(r, "www.myapp.com", trustedPeer)
	if got := r.WildcardDomain(); got != "" {
		t.Fatalf("wildcard inferred from 2 samples: %q", got)
	}

	visit(r, "api.myapp.com", trustedPeer)
	if got := r.WildcardDomain(); got != "*.myapp.com" {
		t.Fatalf("WildcardDomain = %q, want *.myapp.com", got)
	}
	if got := r.BaseDomain(); got != "myapp.com" {
		t.Fatalf("BaseDomain = %q, want myapp.com", got)
	}
}

func TestLearningIgnoresRepeatsOfTheSameHost(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, nil)

	for i := 0; i < 5; i++ {
		visit(r, "myapp.com", trustedPeer)
	}
	if got := r.WildcardDomain(); got != "" {
		t.Fatalf("WildcardDomain = %q, want empty for a single stable host", got)
	}
	if got := r.BaseDomain(); got != "myapp.com" {
		t.Fatalf("BaseDomain = %q, want myapp.com", got)
	}
}

func TestLearningWindowExpiry(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, nil)

	visit(r, "myapp.com", trustedPeer)
	visit(r, "www.myapp.com", trustedPeer)
	visit(r, "api.myapp.com", trustedPeer)
	if got := r.WildcardDomain(); got != "*.myapp.com" {
		t.Fatalf("WildcardDomain = %q, want *.myapp.com", got)
	}

	clock = clock.Add(2 * time.Minute)
	if got := r.WildcardDomain(); got != "" {
		t.Fatalf("WildcardDomain = %q, want empty once the samples aged out", got)
	}
	if got := r.BaseDomain(); got != "myapp.com" {
		t.Fatalf("BaseDomain = %q, want the last resolved base", got)
	}
}

func TestLearningIgnoresUntrustedPeers(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, nil)

	visit(r, "myapp.com", untrustedPeer)
	visit(r, "www.myapp.com", untrustedPeer)
	visit(r, "api.myapp.com", untrustedPeer)

	if got := r.WildcardDomain(); got != "" {
		t.Fatalf("WildcardDomain = %q, want empty for untrusted peers", got)
	}
}

func TestLearningDisabled(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, func(o *Options) {
		o.Learning.Enabled = Bool(false)
	})

	visit(r, "myapp.com", trustedPeer)
	visit(r, "www.myapp.com", trustedPeer)
	visit(r, "api.myapp.com", trustedPeer)

	if got := r.WildcardDomain(); got != "" {
		t.Fatalf("WildcardDomain = %q, want empty when learning is off", got)
	}
}

func TestLearningDefaultsAreApplied(t *testing.T) {
	isolateEnv(t)

	opts := baseOptions()
	opts.Domain = ""
	opts.Learning = LearningOptions{}
	r := New(opts)

	if !r.learning || !r.logChanges || !r.liveReload {
		t.Fatalf("learning switches default to true, got %v %v %v", r.learning, r.logChanges, r.liveReload)
	}
	if r.minSamples != 3 {
		t.Fatalf("minSamples = %d, want 3", r.minSamples)
	}
	if r.sampleWindow != 5*time.Minute {
		t.Fatalf("sampleWindow = %s, want 5m", r.sampleWindow)
	}
}

func TestConfiguredDomainListSkipsLearning(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, func(o *Options) {
		o.Domain = "myapp.com,www.myapp.com,api.myapp.com"
	})

	visit(r, "other.example.org", trustedPeer)
	visit(r, "www.example.org", trustedPeer)
	visit(r, "api.example.org", trustedPeer)

	if got := r.BaseDomain(); got != "myapp.com" {
		t.Fatalf("BaseDomain = %q, want the configured primary domain", got)
	}
	if got := r.WildcardDomain(); got != "*.myapp.com" {
		t.Fatalf("WildcardDomain = %q, want *.myapp.com", got)
	}
}

func TestSingleConfiguredDomainHasNoWildcard(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, func(o *Options) {
		o.Domain = "myapp.com"
	})

	if got := r.WildcardDomain(); got != "" {
		t.Fatalf("WildcardDomain = %q, want empty for a single configured domain", got)
	}
	if got := r.BaseDomain(); got != "myapp.com" {
		t.Fatalf("BaseDomain = %q, want myapp.com", got)
	}
}

func TestOnChangeAndLogging(t *testing.T) {
	isolateEnv(t)

	type change struct{ old, updated string }

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	changes := []change{}
	logged := 0
	r := learningResolver(t, &clock, func(o *Options) {
		o.OnChange = func(oldFQDN, newFQDN string) {
			changes = append(changes, change{old: oldFQDN, updated: newFQDN})
		}
		o.Logf = func(format string, args ...any) {
			logged++
		}
	})

	visit(r, "a.example.org", trustedPeer)
	visit(r, "a.example.org", trustedPeer)
	visit(r, "b.example.org", trustedPeer)

	want := []change{
		{old: "", updated: "a.example.org"},
		{old: "a.example.org", updated: "b.example.org"},
	}
	if len(changes) != len(want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("change %d = %v, want %v", i, changes[i], want[i])
		}
	}
	if logged != len(want) {
		t.Fatalf("logged %d change lines, want %d", logged, len(want))
	}
}

func TestLogChangesDisabled(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	logged := 0
	r := learningResolver(t, &clock, func(o *Options) {
		o.Learning.LogChanges = Bool(false)
		o.Logf = func(format string, args ...any) {
			logged++
		}
	})

	visit(r, "a.example.org", trustedPeer)
	visit(r, "b.example.org", trustedPeer)

	if logged != 0 {
		t.Fatalf("logged %d lines, want 0 when log_changes is off", logged)
	}
}

func TestLiveReloadDisabledPinsFirstFQDN(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, func(o *Options) {
		o.Learning.LiveReload = Bool(false)
	})

	if got := visit(r, "a.example.org", trustedPeer); got != "a.example.org" {
		t.Fatalf("first FQDN = %q, want a.example.org", got)
	}
	if got := visit(r, "b.example.org", trustedPeer); got != "a.example.org" {
		t.Fatalf("FQDN = %q, want the pinned a.example.org", got)
	}
}

func TestConcurrentURLVars(t *testing.T) {
	isolateEnv(t)

	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := learningResolver(t, &clock, nil)

	hosts := []string{"one.myapp.com", "two.myapp.com", "three.myapp.com", "myapp.com"}
	done := make(chan struct{})
	for _, host := range hosts {
		go func(h string) {
			for i := 0; i < 50; i++ {
				visit(r, h, trustedPeer)
				r.BaseDomain()
				r.WildcardDomain()
				r.IsTrustedProxy(trustedPeer)
			}
			done <- struct{}{}
		}(host)
	}
	for range hosts {
		<-done
	}

	if got := r.WildcardDomain(); got != "*.myapp.com" {
		t.Fatalf("WildcardDomain = %q, want *.myapp.com", got)
	}
}

func TestDominantBase(t *testing.T) {
	tests := []struct {
		name         string
		hosts        []string
		wantBase     string
		wantDistinct int
	}{
		{name: "empty", hosts: nil, wantBase: "", wantDistinct: 0},
		{name: "ips only", hosts: []string{"192.0.2.1", "192.0.2.2"}, wantBase: "", wantDistinct: 0},
		{
			name:         "most distinct hosts wins",
			hosts:        []string{"a.one.com", "b.one.com", "two.com", "two.com", "two.com"},
			wantBase:     "one.com",
			wantDistinct: 2,
		},
		{
			name:         "ties broken by observation count",
			hosts:        []string{"a.one.com", "b.two.com", "b.two.com"},
			wantBase:     "two.com",
			wantDistinct: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base, distinct := dominantBase(tc.hosts)
			if base != tc.wantBase || distinct != tc.wantDistinct {
				t.Fatalf("dominantBase = (%q, %d), want (%q, %d)", base, distinct, tc.wantBase, tc.wantDistinct)
			}
		})
	}
}
