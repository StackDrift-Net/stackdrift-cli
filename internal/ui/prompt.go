package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

var reader = bufio.NewReader(os.Stdin)

// Out is where the CLI writes. Swapped in tests, which otherwise have no way to
// read back what the user was actually told.
var Out io.Writer = os.Stdout

// Interactive reports whether there is a person to answer.
//
// Ask discards the read error, so a closed or piped stdin comes back as an
// empty line and Confirm reads that as the default. Anything whose default is
// yes therefore has to check first, or a scripted run answers on the user's
// behalf.
func Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func Ask(prompt string) string {
	fmt.Fprint(Out, prompt)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func Confirm(prompt string, def bool) bool {
	suffix := " [y/N] "
	if def {
		suffix = " [Y/n] "
	}
	answer := strings.ToLower(Ask(prompt + suffix))
	if answer == "" {
		return def
	}
	return answer == "y" || answer == "yes"
}

func AskInt(prompt string, min, max int) (int, bool) {
	answer := Ask(prompt)
	if answer == "" {
		return 0, false
	}
	value, err := strconv.Atoi(answer)
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}

func Println(args ...any) {
	fmt.Fprintln(Out, args...)
}

func Printf(format string, args ...any) {
	fmt.Fprintf(Out, format, args...)
}
