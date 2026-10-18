//go:build !windows

package setup

// DetectGitBash: Git for Windows exists only on Windows.
func DetectGitBash() string { return "" }
