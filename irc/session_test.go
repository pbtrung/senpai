package irc

import (
	"strings"
	"testing"
	"time"
)

func testSession(t *testing.T) (*Session, chan Message) {
	t.Helper()
	out := make(chan Message, 4096)
	s := newSession(out, SessionParams{Nickname: "alice", Username: "alice"})
	s.registered = true
	t.Cleanup(s.Close)
	return s, out
}

func handleLine(t *testing.T, s *Session, line string) Event {
	t.Helper()
	msg, err := ParseMessage(line)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := s.HandleMessage(msg)
	if err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	return ev
}

func TestMultilineHistoryAndSearch(t *testing.T) {
	for _, batchType := range []string{"chathistory #c", "soju.im/search"} {
		t.Run(batchType, func(t *testing.T) {
			s, _ := testSession(t)
			handleLine(t, s, "@draft/chathistory-end :server BATCH +h "+batchType)
			for _, line := range []string{
				"@batch=h;time=2026-10-24T12:00:00.000Z;msgid=one :bob!u@h BATCH +m draft/multiline #c",
				"@batch=m :bob!u@h PRIVMSG #c :hello",
				"@batch=m;draft/multiline-concat :bob!u@h PRIVMSG #c : world",
				"@batch=m :bob!u@h PRIVMSG #c :",
				"@batch=m :bob!u@h PRIVMSG #c :second line",
				":server BATCH -m",
			} {
				if ev := handleLine(t, s, line); ev != nil {
					t.Fatalf("premature event: %#v", ev)
				}
			}
			ev := handleLine(t, s, ":server BATCH -h")
			var message MessageEvent
			switch ev := ev.(type) {
			case HistoryEvent:
				if !ev.End || len(ev.Messages) != 1 {
					t.Fatalf("bad history: %#v", ev)
				}
				message = ev.Messages[0].(MessageEvent)
			case SearchEvent:
				if len(ev.Messages) != 1 {
					t.Fatalf("bad search: %#v", ev)
				}
				message = ev.Messages[0]
			default:
				t.Fatalf("unexpected event: %#v", ev)
			}
			if message.Content != "hello world\n\nsecond line" || message.Time.Format(time.RFC3339) != "2026-10-24T12:00:00Z" {
				t.Fatalf("bad assembled message: %#v", message)
			}
		})
	}
}

func TestMultilineSendLimitsAndFallback(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		s, out := testSession(t)
		if enabled {
			s.enabledCaps["draft/multiline"] = struct{}{}
		}
		s.availableCaps["draft/multiline"] = "max-bytes=8,max-lines=2"
		s.sendText("PRIVMSG", "#c", "hello\n\n世界world\r\nend", 8)
		var refs int
		for len(out) > 0 {
			m := <-out
			wire := m.String()
			if strings.ContainsAny(wire, "\r\n") {
				t.Fatalf("wire injection: %q", wire)
			}
			parsed, err := ParseMessage(wire)
			if err != nil || len(parsed.Params) != len(m.Params) {
				t.Fatalf("bad serialization: %q", wire)
			}
			if m.Command == "BATCH" && strings.HasPrefix(m.Params[0], "+") {
				refs++
			}
			if !enabled && m.Command != "PRIVMSG" {
				t.Fatalf("batch without capability: %#v", m)
			}
		}
		if enabled && refs < 2 {
			t.Fatalf("limits did not split batches: %d", refs)
		}
	}
}

func TestMultilineInvalidFragment(t *testing.T) {
	s, _ := testSession(t)
	handleLine(t, s, ":bob BATCH +m draft/multiline #c")
	handleLine(t, s, "@batch=m :bob NOTICE #wrong :bad")
	msg, _ := ParseMessage(":server BATCH -m")
	if _, err := s.HandleMessage(msg); err == nil {
		t.Fatal("accepted mismatched target")
	}
	if len(s.multilineBatches) != 0 {
		t.Fatal("failed batch was retained")
	}
}

func TestUnreadTargets(t *testing.T) {
	s, out := testSession(t)
	s.enabledCaps["soju.im/unread"] = struct{}{}
	s.enabledCaps["draft/chathistory"] = struct{}{}
	s.NewHistoryRequest("").WithLimit(1000).Targets(time.Now(), time.Unix(0, 0))
	if m := <-out; m.Params[3] != "1000" {
		t.Fatalf("target limit capped to message limit: %#v", m)
	}
	handleLine(t, s, ":server BATCH +t draft/chathistory-targets")
	handleLine(t, s, "@batch=t;soju.im/unread;soju.im/mention :server CHATHISTORY TARGETS #c 2026-10-24T12:00:00.000Z")
	handleLine(t, s, "@batch=t :server CHATHISTORY TARGETS bob 2026-10-24T12:00:00.000Z")
	ev := handleLine(t, s, ":server BATCH -t").(HistoryTargetsEvent)
	if !ev.Unread["#c"].Unread || !ev.Unread["#c"].Mention || ev.Unread["bob"].Unread {
		t.Fatalf("bad unread snapshot: %#v", ev)
	}
}

func TestVirtualSessionIsolation(t *testing.T) {
	s, out := testSession(t)
	s.enabledCaps["soju.im/multiplex"] = struct{}{}
	s.enabledCaps["labeled-response"] = struct{}{}
	s.enabledCaps["soju.im/bouncer-networks-notify"] = struct{}{}
	a, b := s.NewVirtualSession("1"), s.NewVirtualSession("2")
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	if a.HasCapability("soju.im/multiplex") || a.HasCapability("soju.im/bouncer-networks-notify") {
		t.Fatal("inherited root-only capabilities")
	}
	for _, session := range []*Session{a, b} {
		handleLine(t, session, ":server BATCH +same chathistory #c")
		handleLine(t, session, "@batch=same :bob PRIVMSG #c :"+session.NetID())
	}
	for _, session := range []*Session{a, b} {
		ev := handleLine(t, session, ":server BATCH -same").(HistoryEvent)
		if ev.Messages[0].(MessageEvent).Content != session.NetID() {
			t.Fatal("cross-network batch leakage")
		}
	}
	a.Close()
	b.PrivMsg("#c", "still connected")
	found := false
	for len(out) > 0 {
		m := <-out
		if m.Command == "PRIVMSG" {
			found = m.Tags[NetworkTag] == "2"
		}
		if m.Command == "CAP" || m.Command == "NICK" || m.Command == "USER" || m.Command == "AUTHENTICATE" {
			t.Fatalf("virtual session attempted registration: %#v", m)
		}
	}
	if !found {
		t.Fatal("closing one session closed shared transport")
	}
}

func TestVirtualDeferredCapabilities(t *testing.T) {
	s, out := testSession(t)
	s.enabledCaps["soju.im/multiplex"] = struct{}{}
	s.availableCaps["soju.im/search"] = ""
	v := s.NewVirtualSession("1")
	t.Cleanup(v.Close)
	found := false
	for len(out) > 0 {
		m := <-out
		if m.Command == "CAP" && m.Params[0] == "REQ" && m.Params[1] == "soju.im/search" && m.Tags[NetworkTag] == "1" {
			found = true
		}
	}
	if !found {
		t.Fatal("virtual session never requested parent's pending capability")
	}
}
