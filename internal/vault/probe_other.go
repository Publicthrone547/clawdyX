//go:build !linux

package vault

// Available reports whether the system secret store can be used; the
// macOS Keychain and the Windows Credential Manager always exist. Tests
// replace it.
var Available = func() bool { return true }
