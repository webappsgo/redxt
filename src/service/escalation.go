package service

import (
	"fmt"
	"os/exec"
)

// Method identifies one way of obtaining elevated privileges.
type Method string

// Escalation methods, named per AI.md PART 24 "Escalation Detection by OS".
const (
	MethodNone      Method = "none" // already root/Administrator
	MethodSudo      Method = "sudo"
	MethodSu        Method = "su"
	MethodPkexec    Method = "pkexec"
	MethodDoas      Method = "doas"
	MethodOsascript Method = "osascript"
	MethodRunas     Method = "runas"
	MethodUAC       Method = "uac"
)

// PathLookup resolves whether an external command is available on PATH.
// Production code uses osPathLookup (exec.LookPath); tests inject a fake
// so escalation detection never actually probes the host.
type PathLookup interface {
	LookPath(name string) (string, error)
}

// osPathLookup is the production PathLookup backed by exec.LookPath.
type osPathLookup struct{}

// NewOSPathLookup returns the production PathLookup backed by exec.LookPath.
func NewOSPathLookup() PathLookup { return osPathLookup{} }

func (osPathLookup) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// available reports whether name resolves on PATH via lookup.
func available(lookup PathLookup, name string) bool {
	_, err := lookup.LookPath(name)
	return err == nil
}

// DetectEscalationMethods returns, in the exact priority order AI.md
// PART 24 documents for goos, every escalation method actually usable on
// this host: elevated always comes first, and each subsequent tool is
// included only when it resolves via lookup. Windows never resolves a
// PATH tool for UAC (a GUI prompt), so UAC is included whenever the
// process is not already elevated.
func DetectEscalationMethods(goos string, elevated bool, lookup PathLookup) ([]Method, error) {
	if elevated {
		return []Method{MethodNone}, nil
	}

	var order []Method
	switch goos {
	case "linux":
		order = []Method{MethodSudo, MethodSu, MethodPkexec, MethodDoas}
	case "darwin":
		order = []Method{MethodSudo, MethodOsascript}
	case "freebsd", "openbsd", "netbsd":
		order = []Method{MethodDoas, MethodSudo, MethodSu}
	case "windows":
		return []Method{MethodUAC, MethodRunas}, nil
	default:
		return nil, fmt.Errorf("service: unsupported GOOS %q for escalation detection", goos)
	}

	var methods []Method
	for _, m := range order {
		if available(lookup, string(m)) {
			methods = append(methods, m)
		}
	}
	return methods, nil
}

// DetectEscalation returns the single best escalation method for goos:
// MethodNone if already elevated, otherwise the first method in AI.md
// PART 24's priority order that is actually available. It returns an
// error - never a method to prompt with - when the user cannot escalate
// at all, per PART 24's "never prompts if user cannot escalate" rule.
func DetectEscalation(goos string, elevated bool, lookup PathLookup) (Method, error) {
	methods, err := DetectEscalationMethods(goos, elevated, lookup)
	if err != nil {
		return "", err
	}
	if len(methods) == 0 {
		return "", fmt.Errorf("service: no privilege escalation method available on this host; " +
			"install access to sudo/doas/pkexec (or su) or run as root/Administrator")
	}
	return methods[0], nil
}

// EscalationCommand returns the command line that re-invokes binaryPath
// with args under the given escalation method. It never runs the command;
// callers execute it via a Runner.
func EscalationCommand(method Method, binaryPath string, args []string) ([]string, error) {
	switch method {
	case MethodNone:
		return append([]string{binaryPath}, args...), nil
	case MethodSudo:
		return append([]string{"sudo", binaryPath}, args...), nil
	case MethodSu:
		return []string{"su", "-c", shellJoin(append([]string{binaryPath}, args...))}, nil
	case MethodPkexec:
		return append([]string{"pkexec", binaryPath}, args...), nil
	case MethodDoas:
		return append([]string{"doas", binaryPath}, args...), nil
	case MethodOsascript:
		script := fmt.Sprintf("do shell script %q with administrator privileges", shellJoin(append([]string{binaryPath}, args...)))
		return []string{"osascript", "-e", script}, nil
	case MethodRunas:
		return append([]string{"runas", "/user:Administrator", binaryPath}, args...), nil
	default:
		return nil, fmt.Errorf("service: unsupported escalation method %q", method)
	}
}

// shellJoin quotes each argument for safe inclusion in a single shell
// command string, as required by su -c and osascript's "do shell script".
func shellJoin(args []string) string {
	joined := ""
	for i, a := range args {
		if i > 0 {
			joined += " "
		}
		joined += shellQuote(a)
	}
	return joined
}

// shellQuote wraps a in single quotes, escaping any embedded single quote.
func shellQuote(a string) string {
	out := "'"
	for _, r := range a {
		if r == '\'' {
			out += `'\''`
		} else {
			out += string(r)
		}
	}
	return out + "'"
}
