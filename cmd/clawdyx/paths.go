package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// printPaths is `clawdyx paths [config|env|state|logs|socket]`: where this
// configuration keeps its files. With a name it prints only that path (for
// scripts: tail -f "$(clawdyx paths logs)/bot.log").
func printPaths(out io.Writer, cfgPath, envPath string, args []string) int {
	abs, _ := filepath.Abs(cfgPath)
	state, logs := config.Folders(abs)
	rows := [][2]string{
		{"config", abs},
		{"env", envPath},
		{"state", state},
		{"logs", logs},
		{"socket", filepath.Join(state, "control.sock")},
	}
	if len(args) == 1 {
		for _, r := range rows {
			if r[0] == args[0] {
				fmt.Fprintln(out, r[1])
				return exitOK
			}
		}
		fmt.Fprintln(out, "usage: clawdyx paths [config|env|state|logs|socket]")
		return exitConfig
	}
	for _, r := range rows {
		fmt.Fprintf(out, "%-7s %s\n", r[0], r[1])
	}
	return exitOK
}
