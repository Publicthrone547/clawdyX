//go:build !windows

// Command clawdyxw is the Windows login launcher; it does nothing elsewhere.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "clawdyxw is the Windows login launcher of clawdyx; use clawdyx on this system")
	os.Exit(2)
}
