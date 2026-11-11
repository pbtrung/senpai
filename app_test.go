package senpai

import (
	"git.sr.ht/~delthas/senpai/ui"
	"testing"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
)

func TestMessageNetwork(t *testing.T) {
	m := irc.NewMessage("BATCH", "-same").WithTag(irc.NetworkTag, "2")
	if got := messageNetwork("", m); got != "2" {
		t.Fatalf("tagged message routed to %q", got)
	}
	if got := messageNetwork("1", m); got != "1" {
		t.Fatalf("bound connection routed to %q", got)
	}
	if got := messageNetwork("", irc.NewMessage("PING", "root")); got != "" {
		t.Fatalf("root message routed to %q", got)
	}
}

func TestMultiplexDisconnect(t *testing.T) {
	out := make(chan irc.Message, 1024)
	root := irc.NewSession(out, irc.SessionParams{Nickname: "alice", Username: "alice"})
	ack, _ := irc.ParseMessage(":server CAP alice ACK :soju.im/multiplex")
	if _, err := root.HandleMessage(ack); err != nil {
		t.Fatal(err)
	}
	child := root.NewVirtualSession("1")
	app := App{sessions: map[string]*irc.Session{"": root, "1": child}}
	app.handleIRCEvent("", nil)
	if len(app.sessions) != 0 {
		t.Fatal("disconnected virtual sessions retained")
	}
	// A late UI action must not send on the now-closed physical output channel.
	child.PrivMsg("#c", "late message")
}

func TestBoundsIncludeFirstAndOutOfOrderMessages(t *testing.T) {
	stamp := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	var b bound
	first := ui.Line{At: stamp, Body: ui.PlainString("first")}
	b.Update(&first)
	if !b.first.Equal(stamp) || !b.last.Equal(stamp) || b.Compare(&first) != 0 {
		t.Fatalf("first message excluded from bounds: %#v", b)
	}
	older := ui.Line{At: stamp.Add(-time.Hour), Body: ui.PlainString("older")}
	b.Update(&older)
	if b.Compare(&first) != 0 || b.Compare(&older) != 0 {
		t.Fatal("out-of-order message damaged bounds")
	}
	newer := ui.Line{At: stamp.Add(time.Hour), Body: ui.PlainString("newer")}
	b.Update(&newer)
	if b.Compare(&newer) != 0 || !b.last.Equal(newer.At) {
		t.Fatal("new message excluded from upper bound")
	}
}
