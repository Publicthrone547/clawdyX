package control

// Typed payloads shared by the worker (server) and its clients (desktop app,
// `clawdyx ctl`). Times are unix seconds unless the name says otherwise;
// zero means "never/unknown". Nothing here ever carries a secret.

// Bot polling states.
const (
	BotStopped  = "stopped"
	BotStarting = "starting"
	BotPolling  = "polling"
	BotPaused   = "paused" // polling, but new prompts are refused
	BotStopping = "stopping"
)

// BotState is the polling state of the worker's Telegram front end.
type BotState struct {
	State      string `json:"state"`
	Since      int64  `json:"since"`
	Enabled    bool   `json:"enabled"` // persisted desired state (survives restarts)
	PollStarts int64  `json:"pollStarts"`
	LastError  string `json:"lastError,omitempty"`
}

// TelegramStatus summarises the Bot API connection.
type TelegramStatus struct {
	BotUsername    string `json:"botUsername,omitempty"`
	LastUpdate     int64  `json:"lastUpdate"`
	PollErrors     int64  `json:"pollErrors"`
	SendErrors     int64  `json:"sendErrors"`
	Conflicts409   int64  `json:"conflicts409"`
	LastConflict   int64  `json:"lastConflict"`
	Unauthorized   int64  `json:"unauthorized"`
	UpdatesHandled int64  `json:"updatesHandled"`
	Outbox         int    `json:"outbox"`
	AllowedUsers   int    `json:"allowedUsers"`
	TokenStore     string `json:"tokenStore"`     // "keychain" | "file" | "env"
	TokenHint      string `json:"tokenHint"`      // masked fingerprint, never the token
	WebhookRemoved bool   `json:"webhookRemoved"` // a webhook was found and deleted at start
}

// ClaudeStatus is the non-identifying view of the Claude Code CLI.
type ClaudeStatus struct {
	Binary        string `json:"binary"`
	Version       string `json:"version,omitempty"`
	LoggedIn      *bool  `json:"loggedIn,omitempty"`
	AuthMethod    string `json:"authMethod,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Subscription  string `json:"subscription,omitempty"`
	CheckedAt     int64  `json:"checkedAt"`
	AuthError     string `json:"authError,omitempty"`
	DefaultModel  string `json:"defaultModel"`
	DefaultEffort string `json:"defaultEffort"`
	AutomationOK  bool   `json:"automationEnabled"`
	ScreenControl bool   `json:"screenControl"`
}

// JobCounts summarises active work.
type JobCounts struct {
	Running       int `json:"running"`
	Queued        int `json:"queued"`
	MaxConcurrent int `json:"maxConcurrent"`
}

// Status is the one-screen overview (`status.get`).
type Status struct {
	Version     string         `json:"version"`
	PID         int            `json:"pid"`
	StartedAt   int64          `json:"startedAt"`
	Bot         BotState       `json:"bot"`
	Telegram    TelegramStatus `json:"telegram"`
	Claude      ClaudeStatus   `json:"claude"`
	Active      *SessionInfo   `json:"active,omitempty"`
	Jobs        JobCounts      `json:"jobs"`
	LastError   string         `json:"lastError,omitempty"`
	LastErrorAt int64          `json:"lastErrorAt"`
	Screen      ScreenStatus   `json:"screen"`
}

// ScreenStatus is the state of the screen features.
type ScreenStatus struct {
	Configured bool `json:"configured"` // desktop.screen_control in config.json
	Enabled    bool `json:"enabled"`    // runtime switch (/screen off, GUI switch)
}

// SessionInfo describes one logical session.
type SessionInfo struct {
	ID          int64  `json:"id"`
	Label       string `json:"label"`
	Name        string `json:"name,omitempty"`
	Workspace   string `json:"workspace"`
	Mode        string `json:"mode"`
	Model       string `json:"model"`  // effective
	Effort      string `json:"effort"` // effective
	ModelSet    bool   `json:"modelSet"`
	EffortSet   bool   `json:"effortSet"`
	Status      string `json:"status"`
	Origin      string `json:"origin"`
	ClaudeShort string `json:"claudeShort"`
	Started     bool   `json:"started"`
	Turns       int    `json:"turns"`
	CreatedAt   int64  `json:"createdAt"`
	LastUsedAt  int64  `json:"lastUsedAt"`
	ForgottenAt int64  `json:"forgottenAt,omitempty"`
	DeletedAt   int64  `json:"deletedAt,omitempty"`
	Active      bool   `json:"active"`
	ActiveJobs  int    `json:"activeJobs"`
}

// SessionsListParams selects sessions.
type SessionsListParams struct {
	All       bool `json:"all"`       // include archived
	Forgotten bool `json:"forgotten"` // only the recently forgotten tombstones
}

// SessionCreateParams creates a session.
type SessionCreateParams struct {
	Name      string `json:"name,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// SessionRefParams names a session ("S3", "3", a name or a uuid prefix).
type SessionRefParams struct {
	Ref string `json:"ref"`
}

// SessionRenameParams renames a session.
type SessionRenameParams struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

// SessionForgetParams forgets (and optionally deletes the history of) a session.
type SessionForgetParams struct {
	Ref           string `json:"ref"`
	DeleteHistory bool   `json:"deleteHistory"`
}

// SessionNewForgetParams creates a session and forgets the previous active one.
type SessionNewForgetParams struct {
	Name          string `json:"name,omitempty"`
	DeleteHistory bool   `json:"deleteHistory"`
}

// SessionResult reports a session action and exactly which layers it touched.
type SessionResult struct {
	Session  *SessionInfo `json:"session,omitempty"`
	Previous *SessionInfo `json:"previous,omitempty"`
	Message  string       `json:"message"`
	Deleted  []string     `json:"deleted,omitempty"` // files removed (history deletion)
}

// HistoryInfo describes the Claude Code files of a session.
type HistoryInfo struct {
	Deletable bool     `json:"deletable"`
	Reason    string   `json:"reason,omitempty"`
	Paths     []string `json:"paths"`
	Bytes     int64    `json:"bytes"`
}

// JobInfo describes a job.
type JobInfo struct {
	ID            int64  `json:"id"`
	Label         string `json:"label"`
	SessionID     int64  `json:"sessionId"`
	State         string `json:"state"`
	Kind          string `json:"kind"`
	PID           int    `json:"pid,omitempty"`
	Model         string `json:"model"`
	ResolvedModel string `json:"resolvedModel,omitempty"`
	Effort        string `json:"effort"`
	Mode          string `json:"mode"`
	CreatedAt     int64  `json:"createdAt"`
	StartedAt     int64  `json:"startedAt"`
	FinishedAt    int64  `json:"finishedAt"`
	NumTurns      int    `json:"numTurns"`
	InTokens      int64  `json:"inTokens"`
	OutTokens     int64  `json:"outTokens"`
	ErrorSummary  string `json:"errorSummary,omitempty"`
	ResultPreview string `json:"resultPreview,omitempty"`
	Active        bool   `json:"active"`
}

// JobsListParams selects jobs.
type JobsListParams struct {
	Limit int `json:"limit"`
}

// JobRefParams names a job by number.
type JobRefParams struct {
	ID int64 `json:"id"`
}

// JobEventsParams asks for a job's activity log.
type JobEventsParams struct {
	ID    int64 `json:"id"`
	Limit int   `json:"limit"`
}

// JobEvent is one recorded tool call, denial or retry.
type JobEvent struct {
	TS      int64  `json:"ts"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// CancelResult reports a cancellation.
type CancelResult struct {
	Previous string `json:"previous"`
}

// BotStopParams controls what happens to running jobs.
type BotStopParams struct {
	CancelJobs bool `json:"cancelJobs"`
}

// BotStopResult lists the jobs that were cancelled or left running.
type BotStopResult struct {
	Bot       BotState `json:"bot"`
	Cancelled []string `json:"cancelled"`
	Running   []string `json:"running"`
}

// ProcessRow is a bot-owned Claude child process.
type ProcessRow struct {
	JobID     int64   `json:"jobId"`
	SessionID int64   `json:"sessionId"`
	Session   string  `json:"session"`
	State     string  `json:"state"`
	PID       int     `json:"pid,omitempty"`
	Started   int64   `json:"started"`
	RSSKB     int64   `json:"rssKB"`
	CPU       float64 `json:"cpu"` // percent of one core over the last ~1 s; -1 unknown
	Model     string  `json:"model"`
	Mode      string  `json:"mode"`
	Workspace string  `json:"workspace"`
}

// AgentRow is a Claude Code session reported by `claude agents --json`.
type AgentRow struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	PID     int    `json:"pid,omitempty"`
	Started int64  `json:"started"`
	Dir     string `json:"dir"` // home-relative, shortened
	OwnedBy string `json:"ownedBy,omitempty"`
}

// SystemInfo is a cheap machine summary.
type SystemInfo struct {
	Load1    float64 `json:"load1"`
	Load5    float64 `json:"load5"`
	Load15   float64 `json:"load15"`
	HasLoad  bool    `json:"hasLoad"` // false when the OS has no load average
	MemTotal uint64  `json:"memTotal"`
	MemFree  uint64  `json:"memFree"`
}

// ProcessesInfo is the `processes.list` result.
type ProcessesInfo struct {
	Owned       []ProcessRow `json:"owned"`
	Agents      []AgentRow   `json:"agents"`
	AgentsAt    int64        `json:"agentsAt"`
	AgentsError string       `json:"agentsError,omitempty"`
	ClaudeCount int          `json:"claudeCount"`
	WorkerRSSKB int64        `json:"workerRssKB"`
	System      *SystemInfo  `json:"system,omitempty"`
}

// LogTailParams asks for the last lines of the worker log.
type LogTailParams struct {
	Lines int    `json:"lines"`
	Level string `json:"level,omitempty"` // minimum level: debug|info|warn|error
}

// LogLine is one redacted structured log entry.
type LogLine struct {
	Time  string            `json:"time"`
	Level string            `json:"level"`
	Msg   string            `json:"msg"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

// ModelsInfo lists model choices.
type ModelsInfo struct {
	Builtin       []string    `json:"builtin"`
	Aliases       []AliasInfo `json:"aliases"`
	Verified      []string    `json:"verified"` // names verified for the current CLI version
	EffortLevels  []string    `json:"effortLevels"`
	DefaultModel  string      `json:"defaultModel"`
	DefaultEffort string      `json:"defaultEffort"`
}

// AliasInfo is a custom model alias.
type AliasInfo struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	Canonical string `json:"canonical,omitempty"`
}

// SetModelParams changes a model (session scope, or default when Session is empty and Default is true).
type SetModelParams struct {
	Session string `json:"session,omitempty"`
	Model   string `json:"model"` // name, alias, full ID, or "reset"
	Default bool   `json:"default"`
}

// SetEffortParams changes an effort level.
type SetEffortParams struct {
	Session string `json:"session,omitempty"`
	Effort  string `json:"effort"`
	Default bool   `json:"default"`
}

// SetModeParams changes a session's permission tier.
type SetModeParams struct {
	Session string `json:"session,omitempty"`
	Mode    string `json:"mode"`
}

// SettingResult reports a settings change.
type SettingResult struct {
	Message   string `json:"message"`
	Canonical string `json:"canonical,omitempty"`
}

// DiagnosticCheck is one row of `diagnostics.run`.
type DiagnosticCheck struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Warn   bool   `json:"warn,omitempty"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
	FixURL string `json:"fixUrl,omitempty"` // x-apple.systempreferences: URL the GUI may open
}

// DisplayInfo is one connected display.
type DisplayInfo struct {
	Index   int     `json:"index"`
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	PixelW  int     `json:"pixelW"`
	PixelH  int     `json:"pixelH"`
	PointW  float64 `json:"pointW"`
	PointH  float64 `json:"pointH"`
	Scale   float64 `json:"scale"`
	Main    bool    `json:"main"`
	Builtin bool    `json:"builtin"`
}

// ScreensInfo is the `screens.list` result. Listing never captures.
type ScreensInfo struct {
	Status        ScreenStatus  `json:"status"`
	Displays      []DisplayInfo `json:"displays"`
	Error         string        `json:"error,omitempty"`
	Access        string        `json:"access"` // granted | denied | unknown
	Held          int           `json:"held"`
	CapturesToday int           `json:"capturesToday"`
}

// CaptureParams selects displays: "" or "1" (main), an index, a key prefix, or "all".
type CaptureParams struct {
	Display string `json:"display"`
}

// CaptureInfo is one held capture with its preview JPEG (base64 in JSON).
type CaptureInfo struct {
	ID       string      `json:"id"`
	Display  DisplayInfo `json:"display"`
	TakenAt  int64       `json:"takenAtMs"`
	Preview  []byte      `json:"preview"`
	PreviewW int         `json:"previewW"`
	PreviewH int         `json:"previewH"`
	FullW    int         `json:"fullW"`
	FullH    int         `json:"fullH"`
	Bytes    int         `json:"bytes"`
}

// CaptureResult lists captures and a partial-failure message.
type CaptureResult struct {
	Captures []CaptureInfo `json:"captures"`
	Error    string        `json:"error,omitempty"`
}

// CaptureRefParams names a held capture.
type CaptureRefParams struct {
	ID string `json:"id"`
}

// FullImage is a full-resolution JPEG of a held capture.
type FullImage struct {
	JPEG   []byte `json:"jpeg"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// ScreensUseParams sends a held capture to Claude in a session.
type ScreensUseParams struct {
	ID      string `json:"id"`
	Session string `json:"session,omitempty"` // default: active
	Text    string `json:"text,omitempty"`
}

// SubmitParams queues a prompt from the desktop app.
type SubmitParams struct {
	Session   string `json:"session,omitempty"`
	Text      string `json:"text"`
	CaptureID string `json:"captureId,omitempty"`
}

// SubmitResult reports the queued job.
type SubmitResult struct {
	Job      JobInfo `json:"job"`
	Position int     `json:"position"`
}

// EnabledParams switches a feature.
type EnabledParams struct {
	Enabled bool `json:"enabled"`
}

// AuthorizeParams is sent by Claude's screen tool before each screenshot.
type AuthorizeParams struct {
	Display string `json:"display"`
}

// AllowedUserInfo is one allowlist entry.
type AllowedUserInfo struct {
	ID    int64  `json:"id"`
	Label string `json:"label,omitempty"`
	Role  string `json:"role"`
}

// AllowlistParams replaces the allowlist (the owner entry must stay).
type AllowlistParams struct {
	Users []AllowedUserInfo `json:"users"`
}

// TokenStatus describes the stored bot token without revealing it.
type TokenStatus struct {
	Source      string `json:"source"` // keychain | file | env
	Hint        string `json:"hint"`
	BotUsername string `json:"botUsername,omitempty"`
}

// TokenParams carries a candidate bot token (only over the local socket).
type TokenParams struct {
	Token string `json:"token"`
}

// TokenTestResult reports a getMe check.
type TokenTestResult struct {
	BotUsername string `json:"botUsername"`
	BotID       int64  `json:"botId"`
}

// DetectParams arms sender detection for a number of seconds (max 300).
type DetectParams struct {
	Seconds int `json:"seconds"`
}

// DetectedSender is a private-chat sender seen while detection was armed.
type DetectedSender struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	Username  string `json:"username,omitempty"`
	SeenAt    int64  `json:"seenAt"`
}

// DetectResult lists detected senders.
type DetectResult struct {
	Armed   bool             `json:"armed"`
	Until   int64            `json:"until"`
	Senders []DetectedSender `json:"senders"`
}

// WorkspaceInfo is one configured workspace.
type WorkspaceInfo struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	DefaultMode     string   `json:"defaultMode"`
	AllowedCommands []string `json:"allowedCommands"`
	DeniedCommands  []string `json:"deniedCommands"`
	// IKnowWhatIAmDoing drops the minimum deny rules check for this workspace.
	IKnowWhatIAmDoing bool `json:"iKnowWhatIAmDoing"`
	IsDefault         bool `json:"isDefault"`
}

// SettingsInfo is the editable, non-secret configuration: every key of
// config.json except the Telegram allowlist (telegram.allowlist.*).
type SettingsInfo struct {
	ConfigPath       string          `json:"configPath"`
	OwnerUserID      int64           `json:"ownerUserId"` // read-only here (setup changes it)
	Workspaces       []WorkspaceInfo `json:"workspaces"`
	DefaultWorkspace string          `json:"defaultWorkspace"`

	PollTimeoutSeconds int `json:"pollTimeoutSeconds"`

	ClaudeBinary    string   `json:"claudeBinary"`
	SettingSources  []string `json:"settingSources"` // of user, project, local
	StrictMCPConfig bool     `json:"strictMcpConfig"`
	GitBashPath     string   `json:"gitBashPath"` // Windows only
	DefaultModel    string   `json:"defaultModel"`
	DefaultEffort   string   `json:"defaultEffort"`

	MaxConcurrentJobs  int      `json:"maxConcurrentJobs"`
	JobTimeoutMinutes  int      `json:"jobTimeoutMinutes"`
	MaxTurns           int      `json:"maxTurns"`
	MaxBudgetUSD       float64  `json:"maxBudgetUsd"` // 0 = off
	MaxQueuePerSession int      `json:"maxQueuePerSession"`
	MaxOutputBytes     int      `json:"maxOutputBytes"`
	MaxChildRSSMB      int      `json:"maxChildRssMb"`
	BgWaitCeilingMS    int      `json:"bgWaitCeilingMs"`
	ChildPath          []string `json:"childPath"`        // folders, in order
	ChildPathDefault   []string `json:"childPathDefault"` // what "Reset to default" gives

	ExtraDisallowedTools []string `json:"extraDisallowedTools"`
	// BuiltinDenied is what every job on this OS is denied before any rule
	// of a workspace (read-only).
	BuiltinDenied []string `json:"builtinDenied"`

	AutomationEnabled bool   `json:"automationEnabled"`
	ScreenControl     bool   `json:"screenControl"`
	Camera            bool   `json:"camera"`
	CameraSupported   bool   `json:"cameraSupported"`
	OpenURLs          bool   `json:"openUrls"`
	BrowserApp        string `json:"browserApp"`
	ShutdownCommand   bool   `json:"shutdownCommand"`

	StateDir             string `json:"stateDir"`
	LogDir               string `json:"logDir"`
	LogLevel             string `json:"logLevel"`
	JobsDays             int    `json:"jobsDays"`
	JobEventsDays        int    `json:"jobEventsDays"`
	AuditDays            int    `json:"auditDays"`
	ProcessedUpdatesDays int    `json:"processedUpdatesDays"`

	RestartPending bool `json:"restartPending"`
	// Invalid is set by the app when it read the file itself (the worker is
	// down) and the file does not pass validation: the reason.
	Invalid string `json:"invalid,omitempty"`
}

// SettingsPatch changes settings; nil fields are left alone. Changes to
// the settings take effect after the worker restarts.
type SettingsPatch struct {
	PollTimeoutSeconds *int `json:"pollTimeoutSeconds,omitempty"`

	ClaudeBinary    *string   `json:"claudeBinary,omitempty"`
	SettingSources  *[]string `json:"settingSources,omitempty"`
	StrictMCPConfig *bool     `json:"strictMcpConfig,omitempty"`
	GitBashPath     *string   `json:"gitBashPath,omitempty"`

	MaxConcurrentJobs  *int      `json:"maxConcurrentJobs,omitempty"`
	JobTimeoutMinutes  *int      `json:"jobTimeoutMinutes,omitempty"`
	MaxTurns           *int      `json:"maxTurns,omitempty"`
	MaxBudgetUSD       *float64  `json:"maxBudgetUsd,omitempty"`
	MaxQueuePerSession *int      `json:"maxQueuePerSession,omitempty"`
	MaxOutputBytes     *int      `json:"maxOutputBytes,omitempty"`
	MaxChildRSSMB      *int      `json:"maxChildRssMb,omitempty"`
	BgWaitCeilingMS    *int      `json:"bgWaitCeilingMs,omitempty"`
	ChildPath          *[]string `json:"childPath,omitempty"`

	ExtraDisallowedTools *[]string `json:"extraDisallowedTools,omitempty"`

	AutomationEnabled *bool   `json:"automationEnabled,omitempty"`
	ScreenControl     *bool   `json:"screenControl,omitempty"`
	Camera            *bool   `json:"camera,omitempty"`
	OpenURLs          *bool   `json:"openUrls,omitempty"`
	BrowserApp        *string `json:"browserApp,omitempty"`
	ShutdownCommand   *bool   `json:"shutdownCommand,omitempty"`

	StateDir             *string `json:"stateDir,omitempty"`
	LogDir               *string `json:"logDir,omitempty"`
	LogLevel             *string `json:"logLevel,omitempty"`
	JobsDays             *int    `json:"jobsDays,omitempty"`
	JobEventsDays        *int    `json:"jobEventsDays,omitempty"`
	AuditDays            *int    `json:"auditDays,omitempty"`
	ProcessedUpdatesDays *int    `json:"processedUpdatesDays,omitempty"`

	AddWorkspace    *WorkspaceInfo                 `json:"addWorkspace,omitempty"`
	RemoveWorkspace string                         `json:"removeWorkspace,omitempty"`
	RenameWorkspace *RenameParams                  `json:"renameWorkspace,omitempty"`
	WorkspaceModes  map[string]string              `json:"workspaceModes,omitempty"`
	WorkspaceRules  map[string]WorkspaceRulesPatch `json:"workspaceRules,omitempty"`
}

// RenameParams renames a workspace (only while no session uses it).
type RenameParams struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// WorkspaceRulesPatch replaces a workspace's command rules; nil fields are
// left alone.
type WorkspaceRulesPatch struct {
	Allowed           *[]string `json:"allowed,omitempty"`
	Denied            *[]string `json:"denied,omitempty"`
	IKnowWhatIAmDoing *bool     `json:"iKnowWhatIAmDoing,omitempty"`
}

// Settings sections for SettingsResetParams.
const (
	SectionClaudeCode  = "claude-code"
	SectionDefaults    = "defaults"
	SectionLimits      = "limits"
	SectionFeatures    = "features"
	SectionPermissions = "permissions"
	SectionWorkspaces  = "workspaces"
	SectionLogs        = "logs"
	SectionAdvanced    = "advanced"
	SectionAll         = "all"
)

// SettingsResetParams restores the defaults of one section, or of all
// (the Telegram owner and allowlist and the workspaces' names and folders
// are kept: they are identity, not preferences).
type SettingsResetParams struct {
	Section string `json:"section"`
}

// SettingsExport is the settings file as it is (it never holds the token).
type SettingsExport struct {
	JSON     string `json:"json"`
	Filename string `json:"filename"` // suggested name
}

// SettingsImportParams replaces the settings with an exported file.
// Without Apply it only reports what would change.
type SettingsImportParams struct {
	JSON string `json:"json"`
	// AllowOwnerChange accepts a file whose Telegram owner differs.
	AllowOwnerChange bool `json:"allowOwnerChange"`
	Apply            bool `json:"apply"`
}

// SettingsImportResult lists the changed keys ("claude.max_turns: 60 → 80").
type SettingsImportResult struct {
	Changes      []string     `json:"changes"`
	OwnerChanged bool         `json:"ownerChanged"`
	Applied      bool         `json:"applied"`
	Settings     SettingsInfo `json:"settings"`
}

// WorkspaceDefaultParams sets the workspace for new sessions (immediate).
type WorkspaceDefaultParams struct {
	Name string `json:"name"`
}
