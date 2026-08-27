package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunTUIQuitImmediately(t *testing.T) {
	in := strings.NewReader("quit\n")
	var out, errOut bytes.Buffer

	code := RunTUI(NewHTTPClient("", ""), in, &out, &errOut)
	if code != 0 {
		t.Fatalf("RunTUI() = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "interactive mode") {
		t.Error("RunTUI() did not print the menu")
	}
}

func TestRunTUIEOFExits(t *testing.T) {
	in := strings.NewReader("")
	var out, errOut bytes.Buffer

	code := RunTUI(NewHTTPClient("", ""), in, &out, &errOut)
	if code != 0 {
		t.Fatalf("RunTUI() = %d, want 0 on EOF", code)
	}
}

func TestRunTUIUnknownChoice(t *testing.T) {
	in := strings.NewReader("bogus\nquit\n")
	var out, errOut bytes.Buffer

	code := RunTUI(NewHTTPClient("", ""), in, &out, &errOut)
	if code != 0 {
		t.Fatalf("RunTUI() = %d, want 0", code)
	}
	if !strings.Contains(errOut.String(), "unknown choice") {
		t.Error("RunTUI() should report unknown choices")
	}
}
