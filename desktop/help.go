package main

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/Publicthrone547/clawdyX/desktop/host"
	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// The Help screen: Markdown topics embedded in the app, rendered offline
// for this OS. Each topic is a text/template first (see helpData), then
// Markdown. Links are help:<topic> (another topic) or open:<key> (one of
// the app's known links); the view handles both and nothing else.

//go:embed help/*.md
var helpFS embed.FS

// HelpTopic is one entry of the Help screen's list.
type HelpTopic struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// HelpPage is one rendered topic.
type HelpPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	HTML  string `json:"html"`
}

var helpTopics = []HelpTopic{
	{"getting-started", "Getting started"},
	{"telegram", "Telegram"},
	{"sessions", "Sessions and tiers"},
	{"screens", "Screens and camera"},
	{"worker", "Background worker"},
	{"permissions", "Permissions"},
	{"terminal", "Terminal setup"},
	{"privacy", "Data and privacy"},
	{"troubleshooting", "Troubleshooting"},
	{"uninstall", "Uninstall"},
}

// helpPaths are the places a topic names. On this computer they are the
// real ones; for another OS (tests) the defaults of that OS.
type helpPaths struct {
	Config, State, Logs, Worker string
}

// helpData is what a topic template sees.
type helpData struct {
	W                      platform.Words
	Darwin, Windows, Linux bool
	// Session is platform.Session: aqua, windows, x11, wayland, headless.
	Session string
	Paths   helpPaths
	Version string
}

// examplePaths are the default places of an OS (for an OS other than the
// running one, in tests).
func examplePaths(w platform.Words) helpPaths {
	return helpPaths{Config: w.ConfigPathExample, State: w.StatePathExample, Logs: w.LogPathExample, Worker: w.WorkerPathExample}
}

var defaultSessions = map[string]string{"darwin": "aqua", "windows": "windows", "linux": "x11"}

// hostHelpData is the data for this computer.
func hostHelpData() helpData {
	d := newHelpData(platform.W, string(platform.Detect().Session))
	cfg := config.DefaultPath()
	if abs, err := filepath.Abs(cfg); err == nil {
		cfg = abs
	}
	state, logs := config.Folders(cfg)
	d.Paths.Config, d.Paths.State, d.Paths.Logs = cfg, state, logs
	if w := host.AppWorker(); w != "" {
		d.Paths.Worker = w
	}
	return d
}

func newHelpData(w platform.Words, session string) helpData {
	if session == "" {
		session = defaultSessions[w.OS]
	}
	return helpData{
		W: w, Darwin: w.OS == "darwin", Windows: w.OS == "windows", Linux: w.OS == "linux",
		Session: session, Paths: examplePaths(w), Version: version,
	}
}

var helpMarkdown = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))

// renderHelp renders one topic. Raw HTML in the Markdown is dropped
// (goldmark's default), so a page holds only what Markdown produces.
func renderHelp(id string, d helpData) (HelpPage, error) {
	var t *HelpTopic
	for i := range helpTopics {
		if helpTopics[i].ID == id {
			t = &helpTopics[i]
		}
	}
	if t == nil {
		return HelpPage{}, fmt.Errorf("no help topic %q", id)
	}
	src, err := helpFS.ReadFile("help/" + id + ".md")
	if err != nil {
		return HelpPage{}, err
	}
	tmpl, err := template.New(id).Option("missingkey=error").Parse(string(src))
	if err != nil {
		return HelpPage{}, err
	}
	var md bytes.Buffer
	if err := tmpl.Execute(&md, d); err != nil {
		return HelpPage{}, err
	}
	var html bytes.Buffer
	if err := helpMarkdown.Convert(md.Bytes(), &html); err != nil {
		return HelpPage{}, err
	}
	return HelpPage{ID: t.ID, Title: t.Title, HTML: html.String()}, nil
}

// HelpTopics lists the Help screen's topics.
func (s *SystemService) HelpTopics() []HelpTopic { return helpTopics }

// Help renders one Help topic for this computer.
func (s *SystemService) Help(topic string) (HelpPage, error) {
	return renderHelp(topic, hostHelpData())
}
