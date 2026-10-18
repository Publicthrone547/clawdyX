package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

// SetupState tells the UI whether the first-run wizard is needed.
type SetupState struct {
	ConfigExists  bool   `json:"configExists"`
	ConfigError   string `json:"configError,omitempty"`
	TokenStored   bool   `json:"tokenStored"`
	WorkerFound   bool   `json:"workerFound"`
	ClaudeBinary  string `json:"claudeBinary"`
	Browser       string `json:"browser"`
	SuggestFolder string `json:"suggestFolder"`
	ConfigPath    string `json:"configPath"`
}

// ClaudeCheck is the result of checking the Claude Code CLI and its login.
type ClaudeCheck struct {
	Binary       string `json:"binary"`
	Version      string `json:"version"`
	LoggedIn     bool   `json:"loggedIn"`
	Method       string `json:"method"`
	Subscription string `json:"subscription"`
	Error        string `json:"error,omitempty"`
}

// FolderCheck validates a workspace folder.
type FolderCheck struct {
	Path    string `json:"path"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
}

// FinishParams is what the wizard collected.
type FinishParams struct {
	Token        string `json:"token"`
	OwnerID      int64  `json:"ownerId"`
	Workspace    string `json:"workspace"`
	Browser      string `json:"browser"`
	ClaudeBinary string `json:"claudeBinary"`
	Overwrite    bool   `json:"overwrite"`
	StartService bool   `json:"startService"`
}

// FinishResult reports what Finish did.
type FinishResult struct {
	Message string `json:"message"`
	Service string `json:"service,omitempty"`
}

// SetupService runs the first-run wizard's steps. The token stays in this
// process only until Finish stores it in the Keychain.
type SetupService struct {
	app *application.App
}

func (s *SetupService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.app = application.Get()
	return nil
}

// State reports what is configured already.
func (s *SetupService) State() SetupState {
	st := SetupState{ConfigPath: homeRel(config.DefaultPath()), ClaudeBinary: setup.DetectClaude(), Browser: setup.DetectBrowser()}
	if _, err := os.Stat(config.DefaultPath()); err == nil {
		st.ConfigExists = true
		if _, err := loadConfig(); err != nil {
			st.ConfigError = observability.Redact(err.Error())
		}
	}
	if _, err := config.StoredToken(config.DefaultPath()); err == nil {
		st.TokenStored = true
	} else if _, err := config.LoadToken(config.DefaultEnvPath()); err == nil {
		st.TokenStored = true
	}
	_, err := workerBinary()
	st.WorkerFound = err == nil
	home, _ := os.UserHomeDir()
	if p := filepath.Join(home, "projects"); setup.WorkspaceOK(p) == nil {
		st.SuggestFolder = p
	}
	return st
}

// TestToken checks a token with Telegram and returns the bot's @username.
func (s *SetupService) TestToken(token string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return setup.GetMe(ctx, strings.TrimSpace(token))
}

// DetectOwner lists who messaged the bot (private chats) and clears the
// pending updates. It only works while no worker polls this bot.
func (s *SetupService) DetectOwner(token string) ([]setup.Sender, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	senders, _, err := setup.DetectSenders(ctx, strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	if senders == nil {
		senders = []setup.Sender{}
	}
	return senders, nil
}

// CheckClaude runs claude --version and claude auth status (about 30 s).
func (s *SetupService) CheckClaude(binary string) ClaudeCheck {
	if binary == "" {
		binary = setup.DetectClaude()
	}
	cc := ClaudeCheck{Binary: binary}
	if binary == "" {
		cc.Error = "Claude Code was not found. Install it (brew install --cask claude-code), run claude once in Terminal to log in, then check again."
		return cc
	}
	dir, _ := os.UserHomeDir()
	cli := agent.CLI{Binary: binary, Env: agent.ChildEnv(config.DefaultChildPath()+string(os.PathListSeparator)+filepath.Dir(binary), 0), Dir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	v, err := cli.Version(ctx)
	if err != nil {
		cc.Error = "claude --version failed: " + observability.Redact(err.Error())
		return cc
	}
	cc.Version = v
	a, err := cli.AuthStatus(ctx)
	if err != nil {
		cc.Error = "claude auth status failed: " + observability.Redact(err.Error())
		return cc
	}
	cc.LoggedIn, cc.Method, cc.Subscription = a.LoggedIn, a.AuthMethod, a.SubscriptionType
	if !a.LoggedIn {
		cc.Error = "Claude Code is not logged in. Open Terminal, run claude and log in, then check again. clawdyX never logs in for you."
	}
	return cc
}

// ChooseFolder shows a folder picker and validates the choice.
func (s *SetupService) ChooseFolder() (FolderCheck, error) {
	p, err := s.app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
		SetTitle("Choose the project folder Claude may work in").PromptForSingleSelection()
	if err != nil || p == "" {
		return FolderCheck{}, err
	}
	return s.CheckFolder(p), nil
}

// CheckFolder validates a folder path.
func (s *SetupService) CheckFolder(p string) FolderCheck {
	p = filepath.Clean(setup.ExpandHome(strings.TrimSpace(p)))
	fc := FolderCheck{Path: p}
	if err := setup.WorkspaceOK(p); err != nil {
		fc.Error = err.Error()
		return fc
	}
	fc.OK, fc.Warning = true, setup.PrivacyWarning(p)
	return fc
}

// Finish writes config.json (0600), stores the token in the Keychain and,
// when asked, installs and starts the background worker.
func (s *SetupService) Finish(p FinishParams) (FinishResult, error) {
	cfgPath := config.DefaultPath()
	if _, err := os.Stat(cfgPath); err == nil && !p.Overwrite {
		return FinishResult{}, errors.New("a configuration already exists; confirm replacing it")
	}
	tok := strings.TrimSpace(p.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := setup.GetMe(ctx, tok); err != nil {
		return FinishResult{}, errors.New("Telegram rejected the token: " + err.Error())
	}
	if p.OwnerID <= 0 {
		return FinishResult{}, errors.New("your numeric Telegram user ID is required")
	}
	fc := s.CheckFolder(p.Workspace)
	if !fc.OK {
		return FinishResult{}, errors.New(fc.Error)
	}
	bin := p.ClaudeBinary
	if bin == "" {
		bin = setup.DetectClaude()
	}
	if bin == "" {
		return FinishResult{}, errors.New("Claude Code was not found")
	}
	browser := p.Browser
	if browser == "" {
		browser = setup.DetectBrowser()
	}
	if err := setup.WriteConfig(cfgPath, setup.GenerateConfig(p.OwnerID, fc.Path, browser, bin)); err != nil {
		return FinishResult{}, errors.New(observability.Redact(err.Error()))
	}
	if err := config.StoreToken(cfgPath, tok); err != nil {
		return FinishResult{}, errors.New("could not store the token in " + platform.W.TokenStore + ": " + observability.Redact(err.Error()))
	}
	if got, err := config.StoredToken(cfgPath); err != nil || got != tok {
		return FinishResult{}, errors.New("the token could not be read back from " + platform.W.TokenStore)
	}
	_ = os.Remove(config.DefaultEnvPath())
	res := FinishResult{Message: "Saved. The token is in " + platform.W.TokenStore + "; settings are in " + homeRel(cfgPath) + "."}
	if p.StartService {
		out, err := runWorker(40*time.Second, "service", "install")
		if err != nil {
			return res, errors.New("settings saved, but the background worker did not start: " + err.Error())
		}
		res.Service = out
	}
	return res, nil
}
