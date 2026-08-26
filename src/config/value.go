package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Duration is a time.Duration that marshals to and from the
// human-readable forms used throughout server.yml ("30s", "24h",
// "30d", "1h30m"). The Go standard library does not understand the
// "d" (day), "w" (week), or "y" (year) suffixes the spec uses, so
// they are expanded here before handing off to time.ParseDuration.
type Duration time.Duration

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// String renders the duration using Go's canonical duration syntax.
func (d Duration) String() string {
	return time.Duration(d).String()
}

// UnmarshalYAML decodes a scalar duration. A bare integer is
// interpreted as seconds, matching the rate-limit "window: 60" form
// the spec uses.
func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case int:
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	case int64:
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	case float64:
		*d = Duration(time.Duration(v * float64(time.Second)))
		return nil
	case string:
		parsed, err := ParseDuration(v)
		if err != nil {
			return err
		}
		*d = Duration(parsed)
		return nil
	default:
		return fmt.Errorf("config: cannot parse %v as a duration", raw)
	}
}

// MarshalYAML writes the duration back in its human-readable form.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// dayUnits maps the extended duration suffixes the spec uses onto
// their exact durations. Months and years are calendar approximations
// (30 and 365 days), matching the token-expiration table in PART 11.
var dayUnits = []struct {
	suffix string
	unit   time.Duration
}{
	{"y", 365 * 24 * time.Hour},
	{"w", 7 * 24 * time.Hour},
	{"d", 24 * time.Hour},
}

// ParseDuration parses a duration string, accepting the standard Go
// units plus "d", "w", and "y". A bare number is treated as seconds.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("config: empty duration")
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	for _, u := range dayUnits {
		if !strings.HasSuffix(s, u.suffix) {
			continue
		}
		head := strings.TrimSuffix(s, u.suffix)
		if head == "" {
			continue
		}
		n, err := strconv.ParseFloat(head, 64)
		if err != nil {
			continue
		}
		return time.Duration(n * float64(u.unit)), nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("config: invalid duration %q", s)
	}
	return parsed, nil
}

// ByteSize is a byte count that marshals to and from the suffixed
// forms used in server.yml ("10MB", "1GB", "512KB").
type ByteSize int64

// Bytes returns the underlying byte count.
func (b ByteSize) Bytes() int64 {
	return int64(b)
}

// String renders the byte count using the largest whole binary unit.
func (b ByteSize) String() string {
	n := int64(b)
	if n < 0 {
		return "0B"
	}
	units := []struct {
		suffix string
		scale  int64
	}{
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
	}
	for _, u := range units {
		if n >= u.scale && n%u.scale == 0 {
			return strconv.FormatInt(n/u.scale, 10) + u.suffix
		}
	}
	return strconv.FormatInt(n, 10) + "B"
}

// UnmarshalYAML decodes a scalar byte size. A bare integer is a raw
// byte count.
func (b *ByteSize) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case int:
		*b = ByteSize(v)
		return nil
	case int64:
		*b = ByteSize(v)
		return nil
	case float64:
		*b = ByteSize(int64(v))
		return nil
	case string:
		parsed, err := ParseByteSize(v)
		if err != nil {
			return err
		}
		*b = ByteSize(parsed)
		return nil
	default:
		return fmt.Errorf("config: cannot parse %v as a byte size", raw)
	}
}

// MarshalYAML writes the byte size back in its suffixed form.
func (b ByteSize) MarshalYAML() (any, error) {
	return b.String(), nil
}

// byteUnits maps size suffixes onto their multipliers. Both the "MB"
// and "M" spellings are accepted; all are binary (1024-based)
// multiples, which is what the spec's "10MB" body limit means in
// practice.
var byteUnits = []struct {
	suffix string
	scale  int64
}{
	{"TB", 1 << 40},
	{"GB", 1 << 30},
	{"MB", 1 << 20},
	{"KB", 1 << 10},
	{"T", 1 << 40},
	{"G", 1 << 30},
	{"M", 1 << 20},
	{"K", 1 << 10},
	{"B", 1},
}

// ParseByteSize parses a byte-size string such as "10MB". A bare
// number is a raw byte count.
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("config: empty byte size")
	}
	upper := strings.ToUpper(s)
	for _, u := range byteUnits {
		if !strings.HasSuffix(upper, u.suffix) {
			continue
		}
		head := strings.TrimSpace(strings.TrimSuffix(upper, u.suffix))
		if head == "" {
			continue
		}
		n, err := strconv.ParseFloat(head, 64)
		if err != nil {
			return 0, fmt.Errorf("config: invalid byte size %q", s)
		}
		if n < 0 {
			return 0, fmt.Errorf("config: negative byte size %q", s)
		}
		return int64(n * float64(u.scale)), nil
	}
	n, err := strconv.ParseInt(upper, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: invalid byte size %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("config: negative byte size %q", s)
	}
	return n, nil
}
