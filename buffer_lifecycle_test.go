package senpai

import (
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
)

type bufferTestConsole struct{ *strings.Reader }

func (c bufferTestConsole) Write(p []byte) (int, error) { return io.Discard.Write(p) }
func (c bufferTestConsole) Fd() uintptr                 { return 0 }
func (c bufferTestConsole) SetRaw() error               { return nil }
func (c bufferTestConsole) Reset() error                { return nil }
func (c bufferTestConsole) Close() error                { return nil }
func (c bufferTestConsole) Size() (int, int, int, int, error) {
	return 80, 24, 0, 0, nil
}

func TestBufferEntryLifecycle(t *testing.T) {
	win, _, err := ui.New(ui.Config{
		WithConsole:  bufferTestConsole{strings.NewReader("\x1b[?1;2c")},
		NickColWidth: 10, TextMaxWidth: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { win.Exit(); win.Close() })
	win.AddBuffer("", "home", "")
	out := make(chan irc.Message, 4096)
	session := irc.NewSession(out, irc.SessionParams{Nickname: "me", Username: "me"})
	t.Cleanup(session.Close)
	app := &App{
		win: win, cfg: Defaults(), sessions: map[string]*irc.Session{"": session},
		monitor: map[string]map[string]struct{}{"": {}}, messageBounds: make(map[boundKey]bound),
	}
	app.cfg.Transient = true
	stamp := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	app.RestoreBuffers([]BufferState{
		{Name: "alice", Read: stamp}, {Name: "bob", ClosedAt: stamp.Add(time.Minute)}, {Name: "#c"},
	})
	if got := app.restoreNetworkBuffers("", session); !reflect.DeepEqual(got, []string{"#c"}) {
		t.Fatalf("direct connection did not restore saved channels: %v", got)
	}
	if got := win.ReadMarker("", "alice"); !got.Equal(stamp) {
		t.Fatalf("saved read marker not applied to restored query: %v", got)
	}
	hasBuffer := func(name string) bool {
		for i := 0; ; i++ {
			_, title, ok := win.Buffer(i)
			if !ok {
				return false
			}
			if title == name {
				return true
			}
		}
	}
	recv := func(lines ...string) {
		for _, line := range lines {
			msg, err := irc.ParseMessage(line)
			if err != nil {
				t.Fatal(err)
			}
			app.handleIRCEvent("", msg)
		}
	}
	recv(":srv CAP me ACK :soju.im/unread soju.im/bouncer-networks")
	if got := app.restoreNetworkBuffers("", session); len(got) != 0 {
		t.Fatalf("bouncer's saved channels should not be joined by the client: %v", got)
	}
	recv(
		":srv BATCH +targets draft/chathistory-targets",
		"@batch=targets;soju.im/unread;soju.im/mention :srv CHATHISTORY TARGETS bob 2026-11-09T12:00:00.000Z",
		"@batch=targets :srv CHATHISTORY TARGETS NickServ 2026-11-09T12:00:00.000Z",
		":srv BATCH -targets",
	)
	if hasBuffer("bob") || hasBuffer("NickServ") {
		t.Fatal("old snapshots reopened a closed query or a notice-only target")
	}
	recv(
		":srv BATCH +targets draft/chathistory-targets",
		"@batch=targets;soju.im/unread;soju.im/mention :srv CHATHISTORY TARGETS bob 2026-11-09T12:02:00.000Z",
		":srv BATCH -targets",
	)
	if !hasBuffer("bob") {
		t.Fatal("new offline private message did not reopen a closed query")
	}
	app.closeBuffer("", "bob", "")
	if hasBuffer("bob") || !app.targetClosed("", "bob", time.Time{}) {
		t.Fatal("explicit close did not remove and remember the query")
	}
	recv("@time=2026-11-09T12:03:00.000Z :bob!u@h PRIVMSG me :hello")
	if !hasBuffer("bob") || app.targetClosed("", "bob", time.Time{}) {
		t.Fatal("live private message did not reopen a query")
	}
	recv(":me!u@h JOIN #c", ":srv 366 me #c :End of NAMES", ":op!u@h KICK #c me :reason")
	if !hasBuffer("#c") {
		t.Fatal("kick removed the channel conversation")
	}
	app.closeBuffer("", "#c", "")
	if hasBuffer("#c") {
		t.Fatal("explicit close retained a parted channel")
	}
	states := app.BufferStates()
	restored := &App{}
	restored.RestoreBuffers(states)
	if !restored.targetClosed("", "#c", time.Time{}) || restored.targetClosed("", "bob", time.Time{}) {
		t.Fatal("snapshot lost the conversation's open or closed state")
	}
}
