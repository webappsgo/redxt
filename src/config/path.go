package config

import (
	"errors"
	"path"
	"regexp"
	"strings"
)

// Path validation errors from AI.md PART 5 "Path Normalization &
// Validation". These apply globally to every binary and to every
// configuration value, HTTP request path, file path, and API
// parameter that carries a path.
var (
	// ErrPathTraversal reports a "." or ".." traversal attempt.
	ErrPathTraversal = errors.New("path traversal attempt detected")
	// ErrInvalidPath reports characters outside the allowed set.
	ErrInvalidPath = errors.New("invalid path characters")
	// ErrPathTooLong reports a path or segment over its length cap.
	ErrPathTooLong = errors.New("path exceeds maximum length")
)

// Path length caps from AI.md PART 5.
const (
	// MaxPathLength is the largest accepted whole path.
	MaxPathLength = 2048
	// MaxSegmentLength is the largest accepted single segment.
	MaxSegmentLength = 64
)

// validPathSegment matches a permitted path segment: lowercase
// alphanumerics, hyphens, and underscores only.
var validPathSegment = regexp.MustCompile(`^[a-z0-9_-]+$`)

// NormalizePath cleans a path for safe use. It collapses repeated
// slashes, resolves "." and "..", and strips the leading and trailing
// slashes. It returns an empty string for input that cannot be made
// safe.
func NormalizePath(input string) string {
	if input == "" {
		return ""
	}
	cleaned := path.Clean(input)
	cleaned = strings.Trim(cleaned, "/")
	if cleaned == "." || strings.Contains(cleaned, "..") {
		return ""
	}
	return cleaned
}

// ValidatePathSegment checks a single path segment, such as the
// "admin" in "/server/admin/dashboard".
func ValidatePathSegment(segment string) error {
	if segment == "" {
		return ErrInvalidPath
	}
	if segment == "." || segment == ".." {
		return ErrPathTraversal
	}
	if len(segment) > MaxSegmentLength {
		return ErrPathTooLong
	}
	if !validPathSegment.MatchString(segment) {
		return ErrInvalidPath
	}
	return nil
}

// ValidatePath checks an entire path, rejecting traversal attempts
// before normalization so an encoded "/../" can never slip through.
func ValidatePath(p string) error {
	if len(p) > MaxPathLength {
		return ErrPathTooLong
	}
	if strings.Contains(p, "..") {
		return ErrPathTraversal
	}
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg == "" {
			continue
		}
		if err := ValidatePathSegment(seg); err != nil {
			return err
		}
	}
	return nil
}

// SafePath validates a path and returns its normalized form, or an
// error describing why the input was rejected.
func SafePath(input string) (string, error) {
	if err := ValidatePath(input); err != nil {
		return "", err
	}
	return NormalizePath(input), nil
}
