package config

import (
	"errors"
	"strings"
	"testing"
)

// TestNormalizePath covers cleaning only. NormalizePath collapses a
// traversal that stays inside the tree; rejecting ".." outright is
// ValidatePath's job, which is why SafePath validates before it
// normalizes.
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "leading slash stripped", in: "/admin", want: "admin"},
		{name: "trailing slash stripped", in: "/admin/", want: "admin"},
		{name: "repeated slashes collapsed", in: "//admin///panel", want: "admin/panel"},
		{name: "dot segment removed", in: "/admin/./panel", want: "admin/panel"},
		{name: "root becomes empty", in: "/", want: ""},
		{name: "resolvable traversal is collapsed", in: "/admin/../etc", want: "etc"},
		{name: "escaping traversal rejected", in: "../etc", want: ""},
		{name: "bare traversal rejected", in: "..", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizePath(tt.in); got != tt.want {
				t.Fatalf("NormalizePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidatePathSegment(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{name: "plain", in: "admin", want: nil},
		{name: "with hyphen", in: "my-panel", want: nil},
		{name: "with underscore", in: "my_panel", want: nil},
		{name: "with digits", in: "v2", want: nil},
		{name: "empty", in: "", want: ErrInvalidPath},
		{name: "dot", in: ".", want: ErrPathTraversal},
		{name: "double dot", in: "..", want: ErrPathTraversal},
		{name: "uppercase", in: "Admin", want: ErrInvalidPath},
		{name: "space", in: "my panel", want: ErrInvalidPath},
		{name: "slash", in: "a/b", want: ErrInvalidPath},
		{name: "null byte", in: "ad\x00min", want: ErrInvalidPath},
		{name: "too long", in: strings.Repeat("a", MaxSegmentLength+1), want: ErrPathTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathSegment(tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ValidatePathSegment(%q) = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{name: "root", in: "/", want: nil},
		{name: "single segment", in: "/admin", want: nil},
		{name: "nested", in: "/api/v1/server", want: nil},
		{name: "trailing slash", in: "/admin/", want: nil},
		{name: "traversal", in: "/admin/../etc/passwd", want: ErrPathTraversal},
		{name: "hidden traversal", in: "/a/..%2fb", want: ErrPathTraversal},
		{name: "uppercase segment", in: "/Admin", want: ErrInvalidPath},
		{name: "over length", in: "/" + strings.Repeat("a", MaxPathLength), want: ErrPathTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ValidatePath(%q) = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

func TestSafePath(t *testing.T) {
	got, err := SafePath("//api///v1/")
	if err != nil {
		t.Fatalf("SafePath: %v", err)
	}
	if got != "api/v1" {
		t.Fatalf("SafePath = %q, want %q", got, "api/v1")
	}
	if _, err := SafePath("/api/../../etc"); !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("SafePath traversal = %v, want %v", err, ErrPathTraversal)
	}
}
