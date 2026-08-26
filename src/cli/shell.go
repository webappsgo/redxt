package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// completionFlags is the flag list offered by every generated
// completion script. It is derived from the same fixed command set the
// flag parser registers, so completions can never drift from the
// binary.
var completionFlags = []string{
	"--help", "--version", "--status", "--shell", "--mode", "--config",
	"--data", "--cache", "--log", "--backup", "--pid", "--address",
	"--port", "--baseurl", "--daemon", "--debug", "--color", "--lang",
}

// dirFlags take a directory argument and complete against directories.
var dirFlags = []string{"--config", "--data", "--cache", "--log", "--backup"}

// fileFlags take a file argument and complete against files.
var fileFlags = []string{"--pid"}

// enumFlags map a flag to its fixed set of values.
var enumFlags = map[string][]string{
	"--mode":  {"production", "development", "debug"},
	"--color": {"auto", "yes", "no"},
	"--shell": {"completions", "init", "help"},
}

// shells lists every shell the binary can emit integration for, in the
// order they are documented.
var shells = []string{"bash", "zsh", "fish", "sh", "dash", "ksh", "powershell", "pwsh"}

// SupportedShells returns the shells --shell accepts.
func SupportedShells() []string {
	out := make([]string, len(shells))
	copy(out, shells)
	return out
}

// DetectShell resolves the shell name from $SHELL, falling back to bash
// when the variable is unset or holds no usable name.
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

// Completions returns the completion script for shell, naming the
// binary as name. An unsupported shell is an error.
func Completions(shell, name string) (string, error) {
	switch strings.ToLower(shell) {
	case "bash":
		return bashCompletions(name), nil
	case "zsh":
		return zshCompletions(name), nil
	case "fish":
		return fishCompletions(name), nil
	case "sh", "dash", "ksh":
		return posixCompletions(name), nil
	case "powershell", "pwsh":
		return powershellCompletions(name), nil
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

// RunShell executes the --shell subcommand and returns the process exit
// code. shellName may be empty, in which case the shell is detected.
func RunShell(subcommand, shellName, name string, out, errOut io.Writer) int {
	if shellName == "" {
		shellName = DetectShell()
	}

	switch subcommand {
	case "help", "--help", "-h":
		fmt.Fprint(out, ShellHelp(name))
		return 0
	case "completions":
		script, err := Completions(shellName, name)
		if err != nil {
			fmt.Fprintf(errOut, "%s\n", err)
			return 1
		}
		fmt.Fprint(out, script)
		return 0
	case "init":
		line, err := Init(shellName, name)
		if err != nil {
			fmt.Fprintf(errOut, "%s\n", err)
			return 1
		}
		fmt.Fprint(out, line)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown --shell command %q (use completions, init, or help)\n", subcommand)
		return 1
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
func bashCompletions(name string) string {
	fn := "_" + functionName(name) + "_completions"
	var b strings.Builder

	fmt.Fprintf(&b, "# bash completions for %s\n", name)
	fmt.Fprintf(&b, "%s() {\n", fn)
	b.WriteString("  local cur prev\n")
	b.WriteString("  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("  prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n")
	b.WriteString("  case \"$prev\" in\n")
	for _, flag := range sortedEnumFlags() {
		fmt.Fprintf(&b, "    %s) COMPREPLY=($(compgen -W \"%s\" -- \"$cur\")); return 0 ;;\n",
			flag, strings.Join(enumFlags[flag], " "))
	}
	fmt.Fprintf(&b, "    %s) COMPREPLY=($(compgen -d -- \"$cur\")); return 0 ;;\n", strings.Join(dirFlags, "|"))
	fmt.Fprintf(&b, "    %s) COMPREPLY=($(compgen -f -- \"$cur\")); return 0 ;;\n", strings.Join(fileFlags, "|"))
	b.WriteString("  esac\n")
	fmt.Fprintf(&b, "  COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", strings.Join(completionFlags, " "))
	b.WriteString("}\n")
	fmt.Fprintf(&b, "complete -F %s %s\n", fn, name)
	return b.String()
}

// zshCompletions generates a zsh completion script.
func zshCompletions(name string) string {
	fn := "_" + functionName(name)
	var b strings.Builder

	fmt.Fprintf(&b, "#compdef %s\n", name)
	fmt.Fprintf(&b, "%s() {\n", fn)
	b.WriteString("  _arguments -s \\\n")
	b.WriteString("    '(-h --help)'{-h,--help}'[Show help]' \\\n")
	b.WriteString("    '(-v --version)'{-v,--version}'[Show version]' \\\n")
	b.WriteString("    '--status[Show server status and health]' \\\n")
	b.WriteString("    '--shell[Shell integration]:command:(completions init help)' \\\n")
	b.WriteString("    '--mode[Application mode]:mode:(production development debug)' \\\n")
	b.WriteString("    '--config[Config directory]:directory:_files -/' \\\n")
	b.WriteString("    '--data[Data directory]:directory:_files -/' \\\n")
	b.WriteString("    '--cache[Cache directory]:directory:_files -/' \\\n")
	b.WriteString("    '--log[Log directory]:directory:_files -/' \\\n")
	b.WriteString("    '--backup[Backup directory]:directory:_files -/' \\\n")
	b.WriteString("    '--pid[PID file path]:file:_files' \\\n")
	b.WriteString("    '--address[Listen address]:address:' \\\n")
	b.WriteString("    '--port[Listen port]:port:' \\\n")
	b.WriteString("    '--baseurl[URL path prefix]:path:' \\\n")
	b.WriteString("    '--daemon[Run as daemon]' \\\n")
	b.WriteString("    '--debug[Enable debug mode]' \\\n")
	b.WriteString("    '--color[Color output]:color:(auto yes no)' \\\n")
	b.WriteString("    '--lang[Language for output]:code:'\n")
	b.WriteString("}\n")
	fmt.Fprintf(&b, "%s \"$@\"\n", fn)
	return b.String()
}

// fishCompletions generates a fish completion script.
func fishCompletions(name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# fish completions for %s\n", name)
	fmt.Fprintf(&b, "complete -c %s -f\n", name)
	fmt.Fprintf(&b, "complete -c %s -s h -l help -d 'Show help'\n", name)
	fmt.Fprintf(&b, "complete -c %s -s v -l version -d 'Show version'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l status -d 'Show server status and health'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l shell -x -a 'completions init help' -d 'Shell integration'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l mode -x -a 'production development debug' -d 'Application mode'\n", name)
	for _, flag := range dirFlags {
		fmt.Fprintf(&b, "complete -c %s -l %s -x -a '(__fish_complete_directories)' -d 'Directory'\n",
			name, strings.TrimPrefix(flag, "--"))
	}
	for _, flag := range fileFlags {
		fmt.Fprintf(&b, "complete -c %s -l %s -r -d 'File'\n", name, strings.TrimPrefix(flag, "--"))
	}
	fmt.Fprintf(&b, "complete -c %s -l address -x -d 'Listen address'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l port -x -d 'Listen port'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l baseurl -x -d 'URL path prefix'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l daemon -d 'Run as daemon'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l debug -d 'Enable debug mode'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l color -x -a 'auto yes no' -d 'Color output'\n", name)
	fmt.Fprintf(&b, "complete -c %s -l lang -x -d 'Language for output'\n", name)
	return b.String()
}

// posixCompletions generates the basic completion supported by POSIX
// shells, which offer word lists but no per-flag context.
func posixCompletions(name string) string {
	fn := "_" + functionName(name) + "_completions"
	var b strings.Builder

	fmt.Fprintf(&b, "# POSIX completions for %s\n", name)
	fmt.Fprintf(&b, "%s() {\n", fn)
	fmt.Fprintf(&b, "  echo \"%s\"\n", strings.Join(completionFlags, " "))
	b.WriteString("}\n")
	fmt.Fprintf(&b, "%s_FLAGS=\"%s\"\n", strings.ToUpper(functionName(name)), strings.Join(completionFlags, " "))
	return b.String()
}

// powershellCompletions generates a PowerShell argument completer.
func powershellCompletions(name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# PowerShell completions for %s\n", name)
	fmt.Fprintf(&b, "Register-ArgumentCompleter -Native -CommandName %s -ScriptBlock {\n", name)
	b.WriteString("  param($wordToComplete, $commandAst, $cursorPosition)\n")
	fmt.Fprintf(&b, "  @(%s) | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n",
		quotedList(completionFlags))
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

// sortedEnumFlags returns the enum flag names in a stable order, so the
// generated script is byte-identical between runs.
func sortedEnumFlags() []string {
	return []string{"--color", "--mode", "--shell"}
}
