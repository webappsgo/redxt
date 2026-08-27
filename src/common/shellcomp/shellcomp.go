// Package shellcomp implements the shared "--shell completions/init"
// behavior AI.md PART 33 requires of every binary (server, client,
// agent): built-in, byte-stable completion scripts for bash, zsh,
// fish, the POSIX shells, and PowerShell, generated from a single flag
// table so a binary can never ship completions that drift from its own
// flag set.
package shellcomp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Spec describes one binary's completable surface: its full flag list
// (used verbatim for the POSIX and PowerShell word lists), which flags
// take a directory argument, which take a file argument, and which take
// one of a small fixed set of values.
type Spec struct {
	// Flags lists every long flag, "--" included, in help order.
	Flags []string
	// DirFlags are flags whose value completes against directories.
	DirFlags []string
	// FileFlags are flags whose value completes against files.
	FileFlags []string
	// EnumFlags maps a flag to its fixed set of accepted values.
	EnumFlags map[string][]string
}

// shells lists every shell a binary can emit integration for, in the
// order AI.md PART 33 documents them.
var shells = []string{"bash", "zsh", "fish", "sh", "dash", "ksh", "powershell", "pwsh"}

// Shells returns the shells Completions and Init accept.
func Shells() []string {
	out := make([]string, len(shells))
	copy(out, shells)
	return out
}

// DetectShell resolves the shell name from $SHELL, falling back to bash
// when the variable is unset or names no usable shell.
func DetectShell() string {
	path := strings.TrimSpace(os.Getenv("SHELL"))
	if path == "" {
		return "bash"
	}
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return "bash"
	}
	return name
}

// enumOrder returns spec's enum flag names in a stable, sorted order so
// the generated script is byte-identical between runs.
func enumOrder(spec Spec) []string {
	names := make([]string, 0, len(spec.EnumFlags))
	for name := range spec.EnumFlags {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Completions returns the completion script for shell, naming the
// binary as name. An unsupported shell is an error.
func Completions(spec Spec, shell, name string) (string, error) {
	switch strings.ToLower(shell) {
	case "bash":
		return bashCompletions(spec, name), nil
	case "zsh":
		return zshCompletions(spec, name), nil
	case "fish":
		return fishCompletions(spec, name), nil
	case "sh", "dash", "ksh":
		return posixCompletions(spec, name), nil
	case "powershell", "pwsh":
		return powershellCompletions(spec, name), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", shell, strings.Join(shells, ", "))
	}
}

// Init returns the single line a user adds to their shell rc file to
// load the completions for this binary.
func Init(shell, name string) (string, error) {
	lower := strings.ToLower(shell)
	switch lower {
	case "bash", "zsh":
		return fmt.Sprintf("source <(%s --shell completions %s)\n", name, lower), nil
	case "fish":
		return fmt.Sprintf("%s --shell completions fish | source\n", name), nil
	case "sh", "dash", "ksh":
		return fmt.Sprintf("eval \"$(%s --shell completions %s)\"\n", name, lower), nil
	case "powershell", "pwsh":
		return fmt.Sprintf("Invoke-Expression (& %s --shell completions powershell)\n", name), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", shell, strings.Join(shells, ", "))
	}
}

// functionName turns a binary name into an identifier usable as a shell
// function name, because a binary may be renamed to anything.
func functionName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "_completion"
	}
	return b.String()
}

// bashCompletions generates a bash completion script.
func bashCompletions(spec Spec, name string) string {
	fn := "_" + functionName(name) + "_completions"
	var b strings.Builder

	fmt.Fprintf(&b, "# bash completions for %s\n", name)
	fmt.Fprintf(&b, "%s() {\n", fn)
	b.WriteString("  local cur prev\n")
	b.WriteString("  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("  prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n")
	b.WriteString("  case \"$prev\" in\n")
	for _, flag := range enumOrder(spec) {
		fmt.Fprintf(&b, "    %s) COMPREPLY=($(compgen -W \"%s\" -- \"$cur\")); return 0 ;;\n",
			flag, strings.Join(spec.EnumFlags[flag], " "))
	}
	if len(spec.DirFlags) > 0 {
		fmt.Fprintf(&b, "    %s) COMPREPLY=($(compgen -d -- \"$cur\")); return 0 ;;\n", strings.Join(spec.DirFlags, "|"))
	}
	if len(spec.FileFlags) > 0 {
		fmt.Fprintf(&b, "    %s) COMPREPLY=($(compgen -f -- \"$cur\")); return 0 ;;\n", strings.Join(spec.FileFlags, "|"))
	}
	b.WriteString("  esac\n")
	fmt.Fprintf(&b, "  COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", strings.Join(spec.Flags, " "))
	b.WriteString("}\n")
	fmt.Fprintf(&b, "complete -F %s %s\n", fn, name)
	return b.String()
}

// zshCompletions generates a zsh completion script.
func zshCompletions(spec Spec, name string) string {
	fn := "_" + functionName(name)
	var b strings.Builder

	fmt.Fprintf(&b, "#compdef %s\n", name)
	fmt.Fprintf(&b, "%s() {\n", fn)
	b.WriteString("  _arguments -s \\\n")
	for _, flag := range spec.Flags {
		desc := strings.TrimPrefix(flag, "--")
		switch {
		case len(spec.EnumFlags[flag]) > 0:
			fmt.Fprintf(&b, "    '%s[%s]:value:(%s)' \\\n", flag, desc, strings.Join(spec.EnumFlags[flag], " "))
		case contains(spec.DirFlags, flag):
			fmt.Fprintf(&b, "    '%s[%s]:directory:_files -/' \\\n", flag, desc)
		case contains(spec.FileFlags, flag):
			fmt.Fprintf(&b, "    '%s[%s]:file:_files' \\\n", flag, desc)
		default:
			fmt.Fprintf(&b, "    '%s[%s]' \\\n", flag, desc)
		}
	}
	b.WriteString("    '*::arg:->args'\n")
	b.WriteString("}\n")
	fmt.Fprintf(&b, "%s \"$@\"\n", fn)
	return b.String()
}

// fishCompletions generates a fish completion script.
func fishCompletions(spec Spec, name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# fish completions for %s\n", name)
	fmt.Fprintf(&b, "complete -c %s -f\n", name)
	for _, flag := range spec.Flags {
		long := strings.TrimPrefix(flag, "--")
		switch {
		case len(spec.EnumFlags[flag]) > 0:
			fmt.Fprintf(&b, "complete -c %s -l %s -x -a '%s'\n", name, long, strings.Join(spec.EnumFlags[flag], " "))
		case contains(spec.DirFlags, flag):
			fmt.Fprintf(&b, "complete -c %s -l %s -x -a '(__fish_complete_directories)'\n", name, long)
		case contains(spec.FileFlags, flag):
			fmt.Fprintf(&b, "complete -c %s -l %s -r\n", name, long)
		default:
			fmt.Fprintf(&b, "complete -c %s -l %s\n", name, long)
		}
	}
	return b.String()
}

// posixCompletions generates the basic completion supported by POSIX
// shells, which offer word lists but no per-flag context.
func posixCompletions(spec Spec, name string) string {
	fn := "_" + functionName(name) + "_completions"
	var b strings.Builder

	fmt.Fprintf(&b, "# POSIX completions for %s\n", name)
	fmt.Fprintf(&b, "%s() {\n", fn)
	fmt.Fprintf(&b, "  echo \"%s\"\n", strings.Join(spec.Flags, " "))
	b.WriteString("}\n")
	fmt.Fprintf(&b, "%s_FLAGS=\"%s\"\n", strings.ToUpper(functionName(name)), strings.Join(spec.Flags, " "))
	return b.String()
}

// powershellCompletions generates a PowerShell argument completer.
func powershellCompletions(spec Spec, name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# PowerShell completions for %s\n", name)
	fmt.Fprintf(&b, "Register-ArgumentCompleter -Native -CommandName %s -ScriptBlock {\n", name)
	b.WriteString("  param($wordToComplete, $commandAst, $cursorPosition)\n")
	fmt.Fprintf(&b, "  @(%s) | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n", quotedList(spec.Flags))
	b.WriteString("    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_)\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

// quotedList renders values as a PowerShell single-quoted array body.
func quotedList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, "'"+v+"'")
	}
	return strings.Join(quoted, ", ")
}

// contains reports whether list holds value.
func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
