package platform

// Words is the user-facing vocabulary of one operating system. Code outside
// this package never spells an OS-specific name itself: it uses W (the
// running OS) or, in tests, Darwin, Windows and Linux. Every field is set
// for every OS; a field that does not apply holds the neutral wording.
type Words struct {
	OS           string `json:"os"`           // "darwin" | "windows" | "linux"
	OSName       string `json:"osName"`       // "macOS" | "Windows" | "Linux"
	ThisComputer string `json:"thisComputer"` // "this Mac" | "this PC" | "this computer"
	TheComputer  string `json:"theComputer"`  // "the Mac" | "the PC" | "the computer"
	SettingsApp  string `json:"settingsApp"`  // where OS privacy settings live
	FileManager  string `json:"fileManager"`  // "Finder" | "File Explorer" | "file manager"
	Terminal     string `json:"terminal"`     // the app people type commands in
	Shell        string `json:"shell"`        // "zsh" | "PowerShell" | "bash"
	TrayName     string `json:"trayName"`     // "menu-bar item" | "tray icon"

	TokenStore      string `json:"tokenStore"`      // "the macOS Keychain", …
	TokenStoreShort string `json:"tokenStoreShort"` // "Keychain", …

	ServiceKind   string `json:"serviceKind"`   // how the background worker runs
	ServiceLoaded string `json:"serviceLoaded"` // status text when it is registered
	LaunchAtLogin string `json:"launchAtLogin"` // label of the app's login switch
	ShortcutMod   string `json:"shortcutMod"`   // "⌘" | "Ctrl"
	InstallClaude string `json:"installClaude"` // the command that installs Claude Code

	StartAgainHint  string `json:"startAgainHint"`  // after the bot stopped cleanly
	StopServiceHint string `json:"stopServiceHint"` // how to stop the background service
	StartNeedsHint  string `json:"startNeedsHint"`  // what starting it again needs
	ServiceCommands string `json:"serviceCommands"` // short list of the service commands
	SetupNext       string `json:"setupNext"`       // the end of `clawdyx setup`

	DeniedSummary string `json:"deniedSummary"` // what the assisted tier never runs

	ScreenPermission string `json:"screenPermission"` // "" when the OS has none
	InputPermission  string `json:"inputPermission"`  // "" when the OS has none
	CameraPermission string `json:"cameraPermission"` // the setting that gates the camera
	ScreenNeeds      string `json:"screenNeeds"`      // one line under the screen switch
	ScreenRiskNote   string `json:"screenRiskNote"`   // added to the screen-control warning
	CameraRiskNote   string `json:"cameraRiskNote"`   // added to the camera warning
	UpdateHint       string `json:"updateHint"`       // how to install a newer release

	HomeExample string `json:"homeExample"` // "/Users/you" | `C:\Users\you` | "/home/you"
}

// Darwin is the macOS vocabulary.
var Darwin = Words{
	OS: "darwin", OSName: "macOS",
	ThisComputer: "this Mac", TheComputer: "the Mac",
	SettingsApp: "System Settings", FileManager: "Finder", Terminal: "Terminal", Shell: "zsh",
	TrayName:   "menu-bar item",
	TokenStore: "the macOS Keychain", TokenStoreShort: "Keychain",
	ServiceKind: "LaunchAgent", ServiceLoaded: "loaded in launchd",
	LaunchAtLogin: "Open at login (menu-bar item)",
	ShortcutMod:   "⌘",
	InstallClaude: "brew install --cask claude-code",

	StartAgainHint:  "launchd will not restart a clean exit; start it again with: launchctl kickstart gui/$(id -u)/local.clawdyx",
	StopServiceHint: "Stop the service from macOS: launchctl bootout gui/$(id -u)/local.clawdyx",
	StartNeedsHint:  "You will need macOS (launchctl) to start it again.",
	ServiceCommands: "make stop · make status",
	SetupNext:       "make start      run the bot in the background now and at every login\n  make run        or run it in the foreground (Ctrl-C stops it)",

	DeniedSummary: "rm, sudo, git push, curl/wget, brew, web tools",

	ScreenPermission: "Screen Recording", InputPermission: "Accessibility", CameraPermission: "Camera",
	ScreenNeeds:    "Needs macOS permissions.",
	ScreenRiskNote: "macOS will ask for Screen Recording and Accessibility permission.",
	CameraRiskNote: "macOS asks for Camera permission the first time.",
	UpdateHint:     "Download the .dmg from the release page, quit clawdyX, replace it in Applications, then restart the worker.",

	HomeExample: "/Users/you",
}

// Windows is the Windows vocabulary.
var Windows = Words{
	OS: "windows", OSName: "Windows",
	ThisComputer: "this PC", TheComputer: "the PC",
	SettingsApp: "Windows Settings", FileManager: "File Explorer", Terminal: "PowerShell", Shell: "PowerShell",
	TrayName:   "tray icon",
	TokenStore: "the Windows Credential Manager", TokenStoreShort: "Credential Manager",
	ServiceKind: "login task", ServiceLoaded: "login task registered",
	LaunchAtLogin: "Start with Windows",
	ShortcutMod:   "Ctrl",
	InstallClaude: "winget install Anthropic.ClaudeCode",

	StartAgainHint:  "Start it again on the PC with: clawdyx service start (or from the clawdyX app).",
	StopServiceHint: "Stop the service on the PC: clawdyx service stop",
	StartNeedsHint:  "You will need the PC (clawdyx service start or the clawdyX app) to start it again.",
	ServiceCommands: "clawdyx service stop · clawdyx service status",
	SetupNext:       "clawdyx service start   run the bot in the background now and at every login\n  clawdyx run             or run it in the foreground (Ctrl-C stops it)",

	DeniedSummary: "rm, sudo, git push, curl/wget, winget, PowerShell, web tools",

	CameraPermission: "Camera access for desktop apps",
	ScreenNeeds:      "Works without extra permissions.",
	ScreenRiskNote:   "No extra permission is needed; input cannot reach windows that run as administrator.",
	CameraRiskNote:   "Windows asks nothing; camera access for desktop apps must be on in Windows Settings.",
	UpdateHint:       "Download the installer from the release page and run it; it replaces the app and restarts the worker.",

	HomeExample: `C:\Users\you`,
}

// Linux is the Linux vocabulary.
var Linux = Words{
	OS: "linux", OSName: "Linux",
	ThisComputer: "this computer", TheComputer: "the computer",
	SettingsApp: "the system settings", FileManager: "file manager", Terminal: "a terminal", Shell: "bash",
	TrayName:   "tray icon",
	TokenStore: "the system keyring", TokenStoreShort: "keyring",
	ServiceKind: "systemd user service", ServiceLoaded: "systemd unit loaded",
	LaunchAtLogin: "Start on login (desktop autostart)",
	ShortcutMod:   "Ctrl",
	InstallClaude: "curl -fsSL https://claude.ai/install.sh | bash",

	StartAgainHint:  "Start it again on the computer with: systemctl --user start clawdyx (or clawdyx service start)",
	StopServiceHint: "Stop the service on the computer: clawdyx service stop",
	StartNeedsHint:  "You will need the computer (clawdyx service start) to start it again.",
	ServiceCommands: "clawdyx service stop · clawdyx service status",
	SetupNext:       "clawdyx service start   run the bot in the background now and at every login\n  clawdyx run             or run it in the foreground (Ctrl-C stops it)",

	DeniedSummary: "rm, sudo, git push, curl/wget, apt/dnf/pacman, web tools",

	CameraPermission: "access to the camera device (video group)",
	ScreenNeeds:      "Needs an X11 session (Wayland: screenshots only).",
	ScreenRiskNote:   "Needs an X11 session.",
	CameraRiskNote:   "Your user must be in the video group.",
	UpdateHint:       "Download the package for your distribution from the release page.",

	HomeExample: "/home/you",
}

// All lists the vocabularies of every supported OS (for tests and guards).
var All = []Words{Darwin, Windows, Linux}

// ByOS returns the vocabulary for a GOOS value (Linux for unknown values).
func ByOS(goos string) Words {
	for _, w := range All {
		if w.OS == goos {
			return w
		}
	}
	return Linux
}
