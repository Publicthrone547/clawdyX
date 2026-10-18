//go:build !windows

package policy

var platformSecretPaths []string

// ruleAbs writes an absolute path for a permission rule: "//" anchors an
// absolute filesystem path.
func ruleAbs(p string) string { return "/" + p }
