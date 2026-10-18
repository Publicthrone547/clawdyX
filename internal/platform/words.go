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
	NoKeyringNote   string `json:"noKeyringNote"`   // why the token went to a private file

	ServiceKind       string `json:"serviceKind"`       // how the background worker runs
	ServiceLoaded     string `json:"serviceLoaded"`     // status text when it is registered
	LaunchAtLogin     string `json:"launchAtLogin"`     // label of the app's login switch
	ShortcutMod       string `json:"shortcutMod"`       // "⌘" | "Ctrl"
	InstallClaude     string `json:"installClaude"`     // the command that installs Claude Code
	OpenCommand       string `json:"openCommand"`       // the shell command that opens a URL or file
	OpenURLExample    string `json:"openUrlExample"`    // how Claude opens a URL in a shell; {browser} is desktop.browser_app
	ScreenToolTrouble string `json:"screenToolTrouble"` // what a screen tool may report on this OS

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
	// The default places, as Help names them for an OS other than the
	// running one (on this computer Help shows the real paths).
	ConfigPathExample string `json:"configPathExample"`
	StatePathExample  string `json:"statePathExample"`
	LogPathExample    string `json:"logPathExample"`
	WorkerPathExample string `json:"workerPathExample"`

	// Bot and Diagnostics sentences that differ per OS.
	AutomationExamples  string `json:"automationExamples"`  // what the automation tier can run
	ScreenLabel         string `json:"screenLabel"`         // the screenshot status label ("Screen Recording", "Screenshots")
	ScreenDeniedHelp    string `json:"screenDeniedHelp"`    // after a refused capture
	ScreenNote          string `json:"screenNote"`          // told once after the first screenshot ("" when none)
	CameraDeniedStatus  string `json:"cameraDeniedStatus"`  // /cameras when access is refused
	CameraDeniedFix     string `json:"cameraDeniedFix"`     // Diagnostics fix when access is refused
	CameraAskNote       string `json:"cameraAskNote"`       // when the OS asks at the first capture ("" when it never asks)
	NoCameraFix         string `json:"noCameraFix"`         // Diagnostics fix when no camera is found
	ScreenGrantNote     string `json:"screenGrantNote"`     // after ScreenDeniedHelp in Diagnostics ("" when none)
	InputUnavailableFix string `json:"inputUnavailableFix"` // why mouse and keyboard tools may not work
}

// Darwin is the macOS vocabulary.
var Darwin = Words{
	OS: "darwin", OSName: "macOS",
	ThisComputer: "this Mac", TheComputer: "the Mac",
	SettingsApp: "System Settings", FileManager: "Finder", Terminal: "Terminal", Shell: "zsh",
	TrayName:   "menu-bar item",
	TokenStore: "the macOS Keychain", TokenStoreShort: "Keychain",
	NoKeyringNote: "The macOS Keychain is not available; the token is kept in a private file (mode 0600) instead.",
	ServiceKind:   "LaunchAgent", ServiceLoaded: "loaded in launchd",
	LaunchAtLogin:     "Open at login (menu-bar item)",
	ShortcutMod:       "⌘",
	InstallClaude:     "brew install --cask claude-code",
	OpenCommand:       "open",
	OpenURLExample:    `open -a "{browser}" "URL"`,
	ScreenToolTrouble: "a missing macOS permission",

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

	HomeExample:       "/Users/you",
	ConfigPathExample: "~/.config/clawdyx/config.json", StatePathExample: "~/Library/Application Support/clawdyx",
	LogPathExample: "~/Library/Logs/clawdyx", WorkerPathExample: "/Applications/clawdyX.app/Contents/Helpers/clawdyx",

	AutomationExamples:  "open apps and files, scripts, osascript…",
	ScreenLabel:         "Screen Recording",
	ScreenDeniedHelp:    "System Settings → Privacy & Security → Screen & System Audio Recording.",
	ScreenNote:          "macOS asks about once a month whether clawdyX may keep capturing the screen. The dialog appears on the Mac itself; until someone answers it there, screenshots from Telegram fail.",
	CameraDeniedStatus:  "Camera permission: not granted to clawdyX. System Settings → Privacy & Security → Camera.",
	CameraDeniedFix:     "Turn on clawdyX in System Settings → Privacy & Security → Camera, then restart the worker.",
	CameraAskNote:       "macOS asks on the Mac the first time you send /camera.",
	NoCameraFix:         "Connect a camera or open the lid of a MacBook.",
	ScreenGrantNote:     "Grant it to the clawdyX worker binary, then restart the worker. macOS ties the grant to the binary, so grant it again after each rebuild unless the binary is signed with a stable identity.",
	InputUnavailableFix: "Grant Accessibility to the clawdyX worker, then restart it.",
}

// Windows is the Windows vocabulary.
var Windows = Words{
	OS: "windows", OSName: "Windows",
	ThisComputer: "this PC", TheComputer: "the PC",
	SettingsApp: "Windows Settings", FileManager: "File Explorer", Terminal: "PowerShell", Shell: "PowerShell",
	TrayName:   "tray icon",
	TokenStore: "the Windows Credential Manager", TokenStoreShort: "Credential Manager",
	NoKeyringNote: "The Windows Credential Manager is not available; the token is kept in a private file instead.",
	ServiceKind:   "login task", ServiceLoaded: "login task registered",
	LaunchAtLogin:     "Start with Windows",
	ShortcutMod:       "Ctrl",
	InstallClaude:     "winget install Anthropic.ClaudeCode",
	OpenCommand:       "start",
	OpenURLExample:    `start "" "URL"`,
	ScreenToolTrouble: "that input was blocked (a window running as administrator, or the lock screen)",

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

	HomeExample:       `C:\Users\you`,
	ConfigPathExample: `%APPDATA%\clawdyx\config.json`, StatePathExample: `%LOCALAPPDATA%\clawdyx`,
	LogPathExample: `%LOCALAPPDATA%\clawdyx\logs`, WorkerPathExample: `%LOCALAPPDATA%\Programs\clawdyX\helpers\clawdyx.exe`,

	AutomationExamples:  "open apps and files, scripts",
	ScreenLabel:         "Screenshots",
	ScreenDeniedHelp:    "The capture was refused; try again while the PC is unlocked.",
	CameraDeniedStatus:  "Camera access for desktop apps is off: Windows Settings → Privacy & security → Camera.",
	CameraDeniedFix:     "Turn on camera access for desktop apps in Windows Settings → Privacy & security → Camera, then try again.",
	NoCameraFix:         "Connect a camera.",
	InputUnavailableFix: "Input cannot reach windows that run as administrator, the lock screen or the sign-in screen.",
}

// Linux is the Linux vocabulary.
var Linux = Words{
	OS: "linux", OSName: "Linux",
	ThisComputer: "this computer", TheComputer: "the computer",
	SettingsApp: "the system settings", FileManager: "file manager", Terminal: "a terminal", Shell: "bash",
	TrayName:   "tray icon",
	TokenStore: "the system keyring", TokenStoreShort: "keyring",
	NoKeyringNote: "No system keyring was found (normal on a server); the token is kept in a private file (mode 0600) instead.",
	ServiceKind:   "systemd user service", ServiceLoaded: "systemd unit loaded",
	LaunchAtLogin:     "Start on login (desktop autostart)",
	ShortcutMod:       "Ctrl",
	InstallClaude:     "curl -fsSL https://claude.ai/install.sh | bash",
	OpenCommand:       "xdg-open",
	OpenURLExample:    `xdg-open "URL"`,
	ScreenToolTrouble: "that it is not available in this session (Wayland, or no graphical session)",

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

	HomeExample:       "/home/you",
	ConfigPathExample: "~/.config/clawdyx/config.json", StatePathExample: "~/.local/state/clawdyx",
	LogPathExample: "~/.local/state/clawdyx/logs", WorkerPathExample: "/usr/bin/clawdyx",

	AutomationExamples:  "open apps and files, scripts",
	ScreenLabel:         "Screenshots",
	ScreenDeniedHelp:    "The desktop refused the capture; on Wayland, answer the screenshot dialog on the computer and try again.",
	CameraDeniedStatus:  "No access to the camera device: add your user to the video group, then log in again.",
	CameraDeniedFix:     "Add your user to the video group (sudo usermod -aG video $USER), log in again, then restart the worker.",
	NoCameraFix:         "Connect a camera.",
	InputUnavailableFix: "Mouse and keyboard tools need an X11 session; on Wayland they are not available yet.",
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
