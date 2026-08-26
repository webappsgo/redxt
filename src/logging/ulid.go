package logging

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"
)

// crockfordAlphabet is Crockford base32, the ULID encoding alphabet.
// It omits I, L, O, and U so an identifier cannot be misread.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ULID layout constants: a 48-bit millisecond timestamp encoded in the
// first 10 characters followed by 80 bits of entropy encoded in the
// remaining 16, for a canonical length of 26 characters.
const (
	ulidLen          = 26
	ulidTimeLen      = 10
	ulidEntropyLen   = 16
	ulidEntropyBytes = 10
	ulidMaxTime      = uint64(1)<<48 - 1
)

// ErrInvalidULID is returned when a string is not a canonical
// 26-character Crockford base32 ULID.
var ErrInvalidULID = errors.New("logging: invalid ULID")

// ulidState guards the monotonic generator. Two identifiers created in
// the same millisecond must still sort in creation order, so the
// second one reuses the previous entropy incremented by one instead of
// drawing fresh random bytes.
var ulidState struct {
	mu      sync.Mutex
	hasLast bool
	lastMS  uint64
	entropy [ulidEntropyBytes]byte
}

// NewULID returns a new ULID for the current time.
func NewULID() string {
	return NewULIDAt(time.Now())
}

// NewULIDAt returns a new ULID whose timestamp component is t
// truncated to milliseconds.
//
// Consecutive calls landing in the same millisecond increment the
// previous entropy rather than re-randomizing it, so the returned
// identifiers sort lexicographically in generation order. In the
// practically unreachable case where the entropy of a single
// millisecond overflows (2^80 identifiers), fresh entropy is drawn and
// ordering within that one millisecond is not guaranteed.
func NewULIDAt(t time.Time) string {
	ms := uint64(0)
	if unix := t.UTC().UnixMilli(); unix > 0 {
		ms = uint64(unix) & ulidMaxTime
	}

	ulidState.mu.Lock()
	defer ulidState.mu.Unlock()

	if ulidState.hasLast && ms == ulidState.lastMS {
		if !incrementEntropy(&ulidState.entropy) {
			randomEntropy(&ulidState.entropy)
		}
	} else {
		randomEntropy(&ulidState.entropy)
	}
	ulidState.hasLast = true
	ulidState.lastMS = ms

	return encodeULID(ms, ulidState.entropy)
}

// ParseULIDTime returns the timestamp encoded in a ULID, at
// millisecond precision and in UTC.
func ParseULIDTime(id string) (time.Time, error) {
	if len(id) != ulidLen {
		return time.Time{}, ErrInvalidULID
	}
	var ms uint64
	for i := 0; i < ulidTimeLen; i++ {
		v, ok := crockfordValue(id[i])
		if !ok {
			return time.Time{}, ErrInvalidULID
		}
		ms = ms<<5 | uint64(v)
	}
	for i := ulidTimeLen; i < ulidLen; i++ {
		if _, ok := crockfordValue(id[i]); !ok {
			return time.Time{}, ErrInvalidULID
		}
	}
	if ms > ulidMaxTime {
		return time.Time{}, ErrInvalidULID
	}
	return time.UnixMilli(int64(ms)).UTC(), nil
}

// randomEntropy fills dst with cryptographically secure random bytes.
// Since Go 1.24 crypto/rand.Read never returns an error, so there is
// no failure path to report here.
func randomEntropy(dst *[ulidEntropyBytes]byte) {
	_, _ = rand.Read(dst[:])
}

// incrementEntropy adds one to the big-endian entropy value and
// reports whether it did so without wrapping past its maximum.
func incrementEntropy(e *[ulidEntropyBytes]byte) bool {
	for i := ulidEntropyBytes - 1; i >= 0; i-- {
		e[i]++
		if e[i] != 0 {
			return true
		}
	}
	return false
}

// encodeULID renders a timestamp and entropy pair as 26 Crockford
// base32 characters, most significant bits first.
func encodeULID(ms uint64, entropy [ulidEntropyBytes]byte) string {
	out := make([]byte, ulidLen)
	for i := ulidTimeLen - 1; i >= 0; i-- {
		out[i] = crockfordAlphabet[ms&0x1f]
		ms >>= 5
	}
	for i := 0; i < ulidEntropyLen; i++ {
		var v byte
		for b := 0; b < 5; b++ {
			bit := i*5 + b
			v = v<<1 | (entropy[bit/8]>>(7-uint(bit%8)))&1
		}
		out[ulidTimeLen+i] = crockfordAlphabet[v]
	}
	return string(out)
}

// crockfordValue decodes one Crockford base32 character. Decoding is
// case-insensitive, matching the Crockford specification; encoding
// always emits upper case.
func crockfordValue(c byte) (byte, bool) {
	if c >= 'a' && c <= 'z' {
		c -= 'a' - 'A'
	}
	idx := strings.IndexByte(crockfordAlphabet, c)
	if idx < 0 {
		return 0, false
	}
	return byte(idx), true
}
