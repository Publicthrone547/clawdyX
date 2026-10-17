package telegram

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

const member int64 = 222222222

func twoUsers(c *config.Config) {
	c.Telegram.AllowedUsers = []config.AllowedUser{{ID: owner, Role: config.RoleOwner}, {ID: member, Role: config.RoleUser, Label: "friend"}}
	c.Desktop.ScreenControl = true
}

func sentTo(h *harness, chat int64, substr string) bool {
	for _, c := range h.tg.sent() {
		if (c.Method == "sendMessage" || c.Method == "editMessageText") && c.Form["chat_id"] == fmt.Sprint(chat) && strings.Contains(c.Form["text"], substr) {
			return true
		}
	}
	return false
}

func waitSentTo(t *testing.T, h *harness, chat int64, substr string) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if sentTo(h, chat, substr) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("nothing containing %q sent to chat %d", substr, chat)
}

func TestMemberRepliesGoToTheirChat(t *testing.T) {
	h := startFull(t, twoUsers, nil, fakeDesk{})
	h.tg.push(msg(member, member, "private", "hello from member"))
	waitSentTo(t, h, member, "Hello world")
	if sentTo(h, owner, "Hello world") {
		t.Fatal("the member's answer reached the owner's chat")
	}
	j, _ := h.st.GetJob(1)
	if j.TgChatID != member {
		t.Fatalf("job chat %d", j.TgChatID)
	}
	h.tg.push(msg(member, member, "private", "/help"))
	waitSentTo(t, h, member, "Sessions are shared with the bot's owner")
	h.tg.push(msg(member, member, "private", "/status"))
	waitSentTo(t, h, member, "clawdyx")
}

func TestMemberCannotUseOwnerCommands(t *testing.T) {
	h := startFull(t, twoUsers, nil, fakeDesk{})
	n := fakeCaptures.Load()
	for _, cmd := range []string{"/mode automation", "/screen 1", "/look", "/screens", "/open example.com", "/model fable --default",
		"/effort high --default", "/forget S1", "/newf --delete", "/stop all", "/doctor", "/processes", "/logs j-1", "/alias set x haiku", "/pause", "/cwd w", "/shutdown"} {
		h.tg.push(msg(member, member, "private", cmd))
	}
	var refused int
	for i := 0; i < 120 && refused < 17; i++ { // replies are paced at about 1/s
		refused = 0
		for _, c := range h.tg.sent() {
			if c.Method == "sendMessage" && c.Form["chat_id"] == fmt.Sprint(member) && strings.Contains(c.Form["text"], "Only the owner can do that") {
				refused++
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if refused != 17 {
		t.Fatalf("refused %d of 17 owner-only commands", refused)
	}
	if fakeCaptures.Load() != n {
		t.Fatal("a member triggered a screen capture")
	}
	if len(photos(h)) != 0 || h.app.jobs.Paused() {
		t.Fatal("owner-only command had an effect")
	}
}

func TestMemberCannotCancelOwnersJob(t *testing.T) {
	h := startFull(t, twoUsers, nil, fakeDesk{})
	h.tg.push(msg(owner, owner, "private", "SLOW job"))
	h.tg.waitFor(t, "j-1")
	h.tg.push(msg(member, member, "private", "/cancel j-1"))
	waitSentTo(t, h, member, "only cancel your own jobs")
	h.tg.push(msg(member, member, "private", "/stop"))
	waitSentTo(t, h, member, "Nothing is running")
	if act := h.app.jobs.ActiveJobs(); len(act) != 1 {
		t.Fatalf("owner's job was stopped: %+v", act)
	}
	cb := callback(member, "c:1:whatever", 5)
	cb["callback_query"].(map[string]any)["message"].(map[string]any)["chat"] = map[string]any{"id": member, "type": "private"}
	h.tg.push(cb)
	time.Sleep(300 * time.Millisecond)
	if act := h.app.jobs.ActiveJobs(); len(act) != 1 {
		t.Fatal("member's cancel button stopped the owner's job")
	}
	h.tg.push(msg(owner, owner, "private", "/cancel j-1"))
	h.tg.waitFor(t, "Cancelling j-1")
}

func TestMemberCannotConfirmOwnersToken(t *testing.T) {
	h := startFull(t, twoUsers, nil, fakeDesk{})
	h.tg.push(msg(owner, owner, "private", "/new a"))
	h.tg.waitFor(t, "created")
	h.tg.push(msg(owner, owner, "private", "/forget S1"))
	c := h.tg.waitFor(t, "Forget S1 from clawdyX?")
	tok := confirmToken(t, c)
	h.tg.push(msg(member, member, "private", "/confirm "+tok))
	waitSentTo(t, h, member, "unknown, expired, or already used")
	if x, _ := h.st.GetSession(1); x.Status != "active" {
		t.Fatal("member confirmed the owner's action")
	}
	h.tg.push(msg(owner, owner, "private", "/confirm "+tok))
	h.tg.waitFor(t, "S1 forgotten")
}

func TestStrangerStillSilentWithTwoUsers(t *testing.T) {
	h := startFull(t, twoUsers, nil, fakeDesk{})
	before := len(h.tg.sent())
	h.tg.push(msg(333333333, 333333333, "private", "/status"))
	h.tg.push(msg(member, owner, "private", "/status")) // member writing in the owner's chat id
	time.Sleep(800 * time.Millisecond)
	for _, c := range h.tg.sent()[before:] {
		if c.Method == "sendMessage" {
			t.Fatalf("reply to an unauthorized update: %v", c.Form)
		}
	}
}
