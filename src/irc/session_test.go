package irc

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func TestFeatureRemovalAndValidation(t *testing.T) {
	s, _ := testSession(t)
	s.updateFeatures([]string{"MONITOR=100", "WHOX", "LINELEN=4096", "SOJU.IM/FILEHOST=https://upload.example", "STATUSMSG=@", "CHATHISTORY=500"})
	s.updateFeatures([]string{"-MONITOR", "-WHOX", "-LINELEN", "-SOJU.IM/FILEHOST", "-STATUSMSG", "-CHATHISTORY"})
	if s.monitor || s.whox || s.linelen != 512 || s.upload != "" || s.statusmsg != "" || s.historyLimit != 100 {
		t.Fatal("removed ISUPPORT features retained")
	}
	s.updateFeatures([]string{"LINELEN=-1", "CHATHISTORY=-100", "PREFIX=oops"})
	if s.linelen != 512 || s.historyLimit != 100 || s.prefixModes != "ov" {
		t.Fatal("invalid ISUPPORT value accepted")
	}
	s.updateFeatures([]string{"PREFIX=(qaohv)~&@%+"})
	if s.prefixModes != "qaohv" || s.prefixSymbols != "~&@%+" {
		t.Fatal("valid extended membership prefixes rejected")
	}
	s.updateFeatures([]string{"MONITOR"})
	if !s.monitor {
		t.Fatal("unlimited MONITOR unsupported")
	}
}

func TestOversizedGraphemeChunks(t *testing.T) {
	text := "a" + strings.Repeat("\u0301", 1000) + "z"
	chunks := splitChunks(text, 100)
	for _, chunk := range chunks {
		if len(chunk) > 100 || chunk == "" || !utf8.ValidString(chunk) {
			t.Fatalf("invalid chunk: %q", chunk)
		}
	}
	if strings.Join(chunks, "") != text {
		t.Fatal("grapheme fallback lost content")
	}
}

func TestFailedHistoryCanRetry(t *testing.T) {
	s, out := testSession(t)
	s.enabledCaps["draft/chathistory"] = struct{}{}
	s.NewHistoryRequest("#c").Latest()
	handleLine(t, s, ":server FAIL CHATHISTORY TEMPORARILY_UNAVAILABLE LATEST #c :Try later")
	s.NewHistoryRequest("#c").Latest()
	if len(out) != 2 {
		t.Fatal("failed request permanently blocked history")
	}
}

func TestVirtualRawMessageTags(t *testing.T) {
	s, out := testSession(t)
	s.enabledCaps["soju.im/multiplex"] = struct{}{}
	v := s.NewVirtualSession("1")
	t.Cleanup(v.Close)
	for len(out) > 0 {
		<-out
	}
	v.SendRaw("@+draft/reply=message-id PRIVMSG #c :reply")
	m := <-out
	parsed, err := ParseMessage(m.String())
	if err != nil || parsed.Command != "PRIVMSG" || parsed.Tags[NetworkTag] != "1" || parsed.Tags["+draft/reply"] != "message-id" {
		t.Fatalf("raw tags lost during multiplex routing: %#v (%v)", parsed, err)
	}
}

func TestShortNumericReplies(t *testing.T) {
	s, _ := testSession(t)
	for n := 0; n <= 999; n++ {
		for _, params := range [][]string{nil, {"alice"}} {
			msg := NewMessage(fmt.Sprintf("%03d", n), params...)
			if _, err := s.HandleMessage(msg); err == nil {
				t.Errorf("accepted short numeric %s with %d parameters", msg.Command, len(params))
			}
		}
	}
}

func TestMembershipChangesAreIdempotent(t *testing.T) {
	s, _ := testSession(t)
	handleLine(t, s, ":alice!u@h JOIN #c")
	handleLine(t, s, ":bob!u@h JOIN #c")
	for _, mode := range []string{"+o", "+o", "-v"} {
		handleLine(t, s, ":op MODE #c "+mode+" bob")
	}
	c := s.channels[s.Casemap("#c")]
	if got := c.Members[s.users[s.Casemap("bob")]].Membership; got != "@" {
		t.Fatalf("duplicate or unrelated mode damaged membership: %q", got)
	}
}

func TestInvalidLinkHopCount(t *testing.T) {
	for _, hops := range []string{"-1", "256", "9223372036854775807"} {
		s, _ := testSession(t)
		msg, err := ParseMessage(":server 364 alice remote local :" + hops + " description")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.HandleMessage(msg); err == nil {
			t.Errorf("accepted hop count %s", hops)
		}
	}
}

func TestMalformedBatchIdentifiers(t *testing.T) {
	s, _ := testSession(t)
	for _, id := range []string{"", "+", "-", "unprefixed"} {
		if _, err := s.HandleMessage(NewMessage("BATCH", id, "draft/chathistory-targets")); err == nil {
			t.Errorf("accepted invalid batch ID %q", id)
		}
	}
	if ev := handleLine(t, s, "@batch= :server CHATHISTORY TARGETS #c 2026-11-09T12:00:00.000Z"); ev != nil {
		t.Fatalf("unopened empty batch emitted event: %#v", ev)
	}
}

func TestNamesIgnoresPrefixOnlyMembers(t *testing.T) {
	s, _ := testSession(t)
	handleLine(t, s, ":alice!u@h JOIN #c")
	handleLine(t, s, ":server 353 alice = #c :@ + @+ bob")
	if got := s.Names("#c"); len(got) != 1 || got[0].Name.Name != "bob" {
		t.Fatalf("malformed members were retained: %#v", got)
	}
}

func FuzzSessionMessage(f *testing.F) {
	for _, seed := range []string{
		"PING token", ":server 001 alice :welcome", ":server 005 alice PREFIX=(ov)@+ :supported",
		"@batch= :server CHATHISTORY TARGETS #c 2026-11-09T12:00:00.000Z",
		":server BATCH +", ":bob PRIVMSG alice :hello", ":server 372", ":bob MODE #c +o bob",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 8192 {
			t.Skip()
		}
		msg, err := ParseMessage(raw)
		if err != nil {
			return
		}
		out := make(chan Message, 4096)
		s := newSession(out, SessionParams{Nickname: "alice", Username: "alice"})
		s.registered = true
		defer s.Close()
		_, _ = s.HandleMessage(msg)
	})
}
