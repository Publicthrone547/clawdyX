//go:build !windows

package telegram

// shortTmp keeps socket paths under the 104-byte sun_path limit of macOS.
const shortTmp = "/tmp"
