package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
)

// Export returns the settings file as it is. It never holds the bot token
// (that lives in the token store or the .env file).
func Export(path string) (control.SettingsExport, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return control.SettingsExport{}, err
	}
	if config.ContainsToken(string(b)) {
		return control.SettingsExport{}, errors.New("the settings file contains something that looks like a bot token; not exported")
	}
	return control.SettingsExport{JSON: string(b), Filename: "clawdyx-settings-" + time.Now().Format("2006-01-02") + ".json"}, nil
}

// Import replaces the settings with an exported document. The document
// must be valid on this computer (config.Parse: workspace folders must
// exist, the claude binary must be found); a document that is not changes
// nothing. Without p.Apply it only lists the changes.
func Import(path string, p control.SettingsImportParams) (control.SettingsImportResult, *config.Config, error) {
	var res control.SettingsImportResult
	if config.ContainsToken(p.JSON) {
		return res, nil, errors.New("the file contains something that looks like a bot token; settings files never do")
	}
	in, err := config.Parse([]byte(p.JSON))
	if err != nil {
		return res, nil, err
	}
	cur, _, err := Load(path)
	if err != nil {
		return res, nil, err
	}
	// Compare like Parse sees both files (it derives the allowlist).
	if users, err := config.NormalizeAllowlist(cur.Telegram.OwnerUserID, cur.Telegram.AllowedUsers); err == nil {
		cur.Telegram.AllowedUsers, cur.Telegram.OwnerUserID = users, users[0].ID
	}
	res.Changes = Diff(cur, in)
	res.OwnerChanged = cur.Telegram.OwnerUserID != in.Telegram.OwnerUserID
	if !p.Apply {
		return res, nil, nil
	}
	if res.OwnerChanged && !p.AllowOwnerChange {
		return res, nil, fmt.Errorf("the file is for another Telegram owner (%d instead of %d); confirm to use it", in.Telegram.OwnerUserID, cur.Telegram.OwnerUserID)
	}
	nc, err := config.Rewrite(path, func(c *config.Config) error {
		*c = *in
		return nil
	})
	if err != nil {
		return res, nil, err
	}
	res.Applied = true
	return res, nc, nil
}

// Diff lists the keys whose values differ between two configurations, as
// "key: old → new", sorted. Workspaces are keyed by name.
func Diff(a, b *config.Config) []string {
	fa, fb := flatten(a), flatten(b)
	keys := map[string]bool{}
	for k := range fa {
		keys[k] = true
	}
	for k := range fb {
		keys[k] = true
	}
	var out []string
	for k := range keys {
		if fa[k] != fb[k] {
			out = append(out, fmt.Sprintf("%s: %s → %s", k, orNone(fa[k]), orNone(fb[k])))
		}
	}
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func flatten(c *config.Config) map[string]string {
	b, _ := json.Marshal(c)
	var doc map[string]any
	_ = json.Unmarshal(b, &doc)
	out := map[string]string{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				walk(join(prefix, k), e)
			}
		case []any:
			named := len(x) > 0
			for _, e := range x {
				if m, ok := e.(map[string]any); !ok || m["name"] == nil {
					named = false
				}
			}
			if named {
				for _, e := range x {
					m := e.(map[string]any)
					walk(fmt.Sprintf("%s[%v]", prefix, m["name"]), m)
				}
				return
			}
			s, _ := json.Marshal(x)
			out[prefix] = string(s)
		default:
			s, _ := json.Marshal(x)
			out[prefix] = strings.Trim(string(s), `"`)
		}
	}
	walk("", doc)
	return out
}

func join(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + "." + k
}
