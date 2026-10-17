package telegram

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

// projectsDir is Claude Code's transcript folder, as reported by
// `claude auth status` (falls back to the documented default).
func (a *App) projectsDir() string {
	if auth, _, _ := a.authSnapshot(); auth != nil && auth.ProjectsDirectory != "" {
		return filepath.Clean(auth.ProjectsDirectory)
	}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

func (a *App) busy() sessions.Busy { return a.sessionBusy }

func historyLine(h sessions.History) string {
	var names []string
	for _, p := range h.Paths {
		names = append(names, homeRel(p, 120))
	}
	return fmt.Sprintf("%s (%d item(s), %s)", strings.Join(names, " and "), len(h.Paths), kb(h.Bytes))
}

func kb(n int64) string {
	if n < 1024*1024 {
		return fmt.Sprintf("%d KB", (n+1023)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

func busyMsg(err error, alt string) string {
	if errors.Is(err, sessions.ErrBusy) {
		return Esc(err.Error()) + ". Stop it first (/stop), or " + alt + "."
	}
	return "⚠️ " + Esc(err.Error())
}

// cmdNewForget: /newf [name] [--delete]
func (a *App) cmdNewForget(m *models.Message, args []string) {
	args, _, del := takeFlag(args, "--delete")
	if len(args) > 1 {
		a.reply(m.ID, "Usage: /newf [name] [--delete]")
		return
	}
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	if del {
		old, _ := a.sess.Active()
		if old == nil {
			a.reply(m.ID, "There is no active session to forget. /new creates one.")
			return
		}
		h := sessions.FindHistory(a.projectsDir(), old)
		if !h.Deletable {
			a.reply(m.ID, "Not done: the history of "+old.Label()+" cannot be deleted: "+Esc(h.Reason)+". /newf without --delete forgets it from clawdyX only.")
			return
		}
		tok := a.confirms.New("newf_delete", strconv.FormatInt(old.ID, 10)+"|"+name)
		a.askConfirm(m.ID, fmt.Sprintf("Create a new session, forget %s, and permanently delete its Claude Code history: %s? This cannot be undone.", old.Label(), historyLine(h)), tok, "Create and delete history", "Cancel")
		return
	}
	x, old, err := a.sess.CreateAndForget(name, a.busy())
	if err != nil {
		a.reply(m.ID, busyMsg(err, "use /new to keep it"))
		return
	}
	a.sessionsChanged("newf")
	msg := fmt.Sprintf("🆕 <b>%s</b> created and active.", x.Label())
	if old != nil {
		msg += fmt.Sprintf(" %s forgotten from clawdyX; its Claude Code history is kept. /sessions restore %s within 30 days undoes it.", old.Label(), old.Label())
	}
	a.reply(m.ID, msg)
}

// cmdClear: /clear — fresh context in a new session with the same settings.
func (a *App) cmdClear(m *models.Message) {
	x, from, err := a.sess.Fresh(a.busy())
	if err != nil {
		a.reply(m.ID, busyMsg(err, "wait for it to finish"))
		return
	}
	a.sessionsChanged("fresh")
	if from == nil {
		a.reply(m.ID, fmt.Sprintf("Fresh context: <b>%s</b> created.", x.Label()))
		return
	}
	a.reply(m.ID, fmt.Sprintf("🧹 Fresh context: <b>%s</b> (same workspace, tier, model and effort as %s). %s is kept in /sessions; nothing was deleted.", x.Label(), from.Label(), from.Label()))
}

// cmdForget: /forget S2 [--delete]
func (a *App) cmdForget(m *models.Message, args []string) {
	args, _, del := takeFlag(args, "--delete")
	if len(args) != 1 {
		a.reply(m.ID, "Usage: /forget S-ID [--delete]")
		return
	}
	x, err := a.sess.Resolve(args[0])
	if err != nil {
		a.reply(m.ID, "No such session.")
		return
	}
	if a.sessionBusy(x.ID) {
		a.reply(m.ID, fmt.Sprintf("%s has a queued or running job. Stop it first (/stop %s).", x.Label(), x.Label()))
		return
	}
	q := fmt.Sprintf("Forget %s from clawdyX? It disappears from /sessions; its Claude Code history is kept and you can restore it within 30 days.", x.Label())
	flag := "0"
	if del {
		h := sessions.FindHistory(a.projectsDir(), x)
		if !h.Deletable {
			a.reply(m.ID, "Not done: the history of "+x.Label()+" cannot be deleted: "+Esc(h.Reason)+". /forget without --delete forgets it from clawdyX only.")
			return
		}
		q = fmt.Sprintf("Forget %s and permanently delete its Claude Code history: %s? This cannot be undone.", x.Label(), historyLine(h))
		flag = "1"
	}
	tok := a.confirms.New("forget", strconv.FormatInt(x.ID, 10)+"|"+flag)
	a.askConfirm(m.ID, q, tok, map[bool]string{true: "Forget and delete history", false: "Forget"}[del], "Cancel")
}

// cmdRename: /rename S2 new-name
func (a *App) cmdRename(m *models.Message, args []string) {
	if len(args) != 2 {
		a.reply(m.ID, "Usage: /rename S-ID new-name")
		return
	}
	x, err := a.sess.Resolve(args[0])
	if err != nil {
		a.reply(m.ID, "No such session.")
		return
	}
	if err := a.sess.Rename(x, args[1]); err != nil {
		a.reply(m.ID, "⚠️ "+Esc(err.Error()))
		return
	}
	a.sessionsChanged("rename")
	a.reply(m.ID, fmt.Sprintf("%s is now called “%s”. Only the label changed.", x.Label(), Esc(args[1])))
}

// forgetSession forgets x and, when del, deletes its history. It reports
// exactly which layers changed.
func (a *App) forgetSession(x *store.Session, del bool) (string, []string, error) {
	if err := a.sess.Forget(x, a.busy()); err != nil {
		return "", nil, err
	}
	a.sessionsChanged("forget")
	if !del {
		return fmt.Sprintf("%s forgotten from clawdyX. Its Claude Code history is kept (restore within 30 days: /sessions restore %s).", x.Label(), x.Label()), nil, nil
	}
	removed, err := sessions.DeleteHistory(a.projectsDir(), x)
	if err != nil {
		return fmt.Sprintf("%s forgotten from clawdyX, but its history was NOT deleted: %s", x.Label(), err.Error()), removed, nil
	}
	_ = a.st.MarkHistoryDeleted(x.ID)
	a.st.Audit("session", "delete_history", x.Label(), fmt.Sprintf("%d paths", len(removed)), 0)
	return fmt.Sprintf("%s forgotten and its Claude Code history deleted (%d item(s)).", x.Label(), len(removed)), removed, nil
}

// confirmLifecycle runs confirmed lifecycle actions.
func (a *App) confirmLifecycle(replyTo int, action, args string) bool {
	switch action {
	case "forget":
		idStr, flag, _ := strings.Cut(args, "|")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		x, err := a.st.GetSession(id)
		if err != nil || x.Status == "forgotten" {
			a.reply(replyTo, "Nothing done: that session is gone or already forgotten.")
			return true
		}
		msg, _, err := a.forgetSession(x, flag == "1")
		if err != nil {
			a.reply(replyTo, busyMsg(err, "try again later"))
			return true
		}
		a.reply(replyTo, Esc(msg))
		return true
	case "newf_delete":
		idStr, name, _ := strings.Cut(args, "|")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		cur, _ := a.sess.Active()
		if cur == nil || cur.ID != id {
			a.reply(replyTo, "Nothing done: the active session changed since you asked.")
			return true
		}
		x, old, err := a.sess.CreateAndForget(name, a.busy())
		if err != nil {
			a.reply(replyTo, busyMsg(err, "use /new to keep it"))
			return true
		}
		a.sessionsChanged("newf")
		msg := fmt.Sprintf("🆕 <b>%s</b> created and active. %s forgotten", x.Label(), old.Label())
		removed, derr := sessions.DeleteHistory(a.projectsDir(), old)
		if derr != nil {
			msg += ", but its history was NOT deleted: " + Esc(derr.Error())
		} else {
			_ = a.st.MarkHistoryDeleted(old.ID)
			a.st.Audit("session", "delete_history", old.Label(), fmt.Sprintf("%d paths", len(removed)), 0)
			msg += fmt.Sprintf(" and its Claude Code history deleted (%d item(s)).", len(removed))
		}
		a.reply(replyTo, msg)
		return true
	}
	return false
}

func (a *App) historyInfo(x *store.Session) control.HistoryInfo {
	h := sessions.FindHistory(a.projectsDir(), x)
	out := control.HistoryInfo{Deletable: h.Deletable, Reason: h.Reason, Bytes: h.Bytes, Paths: []string{}}
	for _, p := range h.Paths {
		out.Paths = append(out.Paths, homeRel(p, 200))
	}
	return out
}
