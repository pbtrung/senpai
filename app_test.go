package senpai

import (
	"testing"

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
