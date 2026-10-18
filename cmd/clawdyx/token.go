package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// tokenCmd is `clawdyx token delete`: removes this configuration's bot
// token from the system token store and its token file (uninstallers call
// it). Other configurations' tokens are separate items and stay.
func tokenCmd(store vault.Store, cfgPath, envPath string, args []string, out io.Writer) int {
	if len(args) != 1 || args[0] != "delete" {
		fmt.Fprintln(out, "usage: clawdyx token delete")
		return exitConfig
	}
	removed := false
	switch err := store.Delete(vault.TokenAccount(cfgPath)); {
	case err == nil:
		fmt.Fprintf(out, "removed the bot token from %s\n", platform.W.TokenStore)
		removed = true
	case errors.Is(err, vault.ErrNotFound), errors.Is(err, vault.ErrUnavailable):
	default:
		fmt.Fprintf(out, "could not remove the token from %s: %v\n", platform.W.TokenStore, err)
		return exitError
	}
	if b, err := os.ReadFile(envPath); err == nil && strings.Contains(string(b), "TELEGRAM_BOT_TOKEN") {
		if err := os.Remove(envPath); err != nil {
			fmt.Fprintf(out, "could not remove %s: %v\n", envPath, err)
			return exitError
		}
		fmt.Fprintf(out, "removed %s\n", envPath)
		removed = true
	}
	if !removed {
		fmt.Fprintln(out, "no stored bot token was found")
	}
	fmt.Fprintln(out, "revoke the token in @BotFather (/revoke) so it cannot be used again")
	return exitOK
}
