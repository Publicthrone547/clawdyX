package policy

import "github.com/Publicthrone547/clawdyX/internal/platform"

// hostOS selects the built-in lists of the running OS. The lists of every
// OS are defined on every OS so the tests check all of them anywhere.
var hostOS = platform.W.OS

// CommonDenied are command rules denied on every OS, in every tier and
// every workspace, in addition to the workspace's own denied_commands
// (which must still contain config.MinimumDenied, so 0.x configs stay
// valid). The deny list is the one control that does not depend on
// Claude Code's classifier.
var CommonDenied = []string{
	"Bash(rm *)", "Bash(rmdir *)", "Bash(sudo *)", "Bash(git push *)", "Bash(git remote *)",
	"Bash(curl *)", "Bash(wget *)", "Bash(claude *)", "Bash(dd *)", "Bash(chmod -R *)", "Bash(chown *)",
	"Bash(ssh *)", "Bash(scp *)", "Bash(nc *)", "Bash(ncat *)", "Bash(crontab *)", "Bash(shutdown *)", "Bash(reboot *)",
}

// osDenied are the command rules each OS adds: its package managers, its
// service and system tools, and on Windows the shells and download tools
// that would get around the Unix names (Claude Code's Bash tool runs in
// Git Bash there, which can start them).
var osDenied = map[string][]string{
	"darwin": {
		"Bash(brew *)", "Bash(launchctl *)", "Bash(defaults write *)", "Bash(security *)", "Bash(csrutil *)", "Bash(diskutil *)", "Bash(tmutil *)",
	},
	"windows": {
		"Bash(powershell *)", "Bash(powershell.exe *)", "Bash(pwsh *)", "Bash(cmd *)", "Bash(cmd.exe *)",
		"Bash(del *)", "Bash(erase *)", "Bash(rd *)", "Bash(Remove-Item *)", "Bash(ri *)",
		"Bash(format *)", "Bash(diskpart *)", "Bash(cipher *)", "Bash(reg *)", "Bash(reg.exe *)", "Bash(regedit *)",
		"Bash(schtasks *)", "Bash(sc *)", "Bash(sc.exe *)", "Bash(net *)", "Bash(net.exe *)", "Bash(netsh *)",
		"Bash(bcdedit *)", "Bash(wmic *)", "Bash(taskkill *)", "Bash(runas *)", "Bash(gsudo *)",
		"Bash(winget *)", "Bash(choco *)", "Bash(scoop *)", "Bash(curl.exe *)", "Bash(certutil *)", "Bash(bitsadmin *)",
		"Bash(Invoke-WebRequest *)", "Bash(iwr *)", "Bash(Invoke-RestMethod *)", "Bash(irm *)", "Bash(Invoke-Expression *)", "Bash(iex *)",
		"Bash(Set-ExecutionPolicy *)", "Bash(Start-Process *)", "Bash(mshta *)", "Bash(wscript *)", "Bash(cscript *)",
		"Bash(rundll32 *)", "Bash(regsvr32 *)", "Bash(msiexec *)",
	},
	"linux": {
		"Bash(su *)", "Bash(doas *)", "Bash(pkexec *)", "Bash(apt *)", "Bash(apt-get *)", "Bash(dnf *)", "Bash(yum *)",
		"Bash(pacman *)", "Bash(zypper *)", "Bash(snap *)", "Bash(flatpak *)", "Bash(systemctl *)", "Bash(service *)",
		"Bash(poweroff *)", "Bash(mkfs *)", "Bash(mkfs.ext4 *)", "Bash(mkfs.xfs *)", "Bash(mount *)", "Bash(umount *)",
		"Bash(loginctl *)", "Bash(useradd *)", "Bash(usermod *)", "Bash(passwd *)",
	},
}

// osDisallowedTools are whole Claude Code tools removed on an OS. On
// Windows the PowerShell tool would run commands without passing any
// Bash(...) rule.
var osDisallowedTools = map[string][]string{
	"windows": {"PowerShell"},
}

// DeniedCommandsFor are the built-in command rules of an OS (CommonDenied
// plus the OS's own), the defaults for a new workspace.
func DeniedCommandsFor(goos string) []string {
	return dedupe(append(append([]string{}, CommonDenied...), osDenied[goos]...))
}

// BuiltinDeniedFor is everything denied on an OS before any workspace
// rule: the command rules and the removed tools.
func BuiltinDeniedFor(goos string) []string {
	return dedupe(append(DeniedCommandsFor(goos), osDisallowedTools[goos]...))
}

// BuiltinDenied is BuiltinDeniedFor the running OS (shown read-only in the
// app's rule editor as "always denied").
func BuiltinDenied() []string { return BuiltinDeniedFor(hostOS) }

// DeniedCommands is DeniedCommandsFor the running OS (new workspaces).
func DeniedCommands() []string { return DeniedCommandsFor(hostOS) }
