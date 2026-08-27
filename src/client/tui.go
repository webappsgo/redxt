package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// RunTUI is the interactive mode launched when redxt-cli is invoked
// with no arguments, per AI.md PART 33 "Commands": "No 'tui' command -
// TUI launches automatically when no arguments provided." It is a
// minimal line-based menu rather than a full styled interface; the
// richer themed TUI (PART 33 "CLI/TUI/GUI Theming") is tracked as a
// follow-up in TODO.AI.md.
func RunTUI(client *HTTPClient, in io.Reader, out, errOut io.Writer) int {
	scanner := bufio.NewScanner(in)

	for {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "redxt-cli - interactive mode")
		fmt.Fprintln(out, "  1) health   - show server health")
		fmt.Fprintln(out, "  2) quit     - exit")
		fmt.Fprint(out, "> ")

		if !scanner.Scan() {
			fmt.Fprintln(out)
			return 0
		}

		switch strings.TrimSpace(scanner.Text()) {
		case "1", "health":
			RunHealth(client, out, errOut)
		case "2", "quit", "exit", "q":
			return 0
		case "":
			// blank line: redisplay the menu
		default:
			fmt.Fprintln(errOut, "unknown choice")
		}
	}
}
