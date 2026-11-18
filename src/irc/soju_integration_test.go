package irc

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSojuRS runs against an externally supplied release binary, never a source
// build. All listeners, accounts and storage are local and disposable.
func TestSojuRS(t *testing.T) {
	binary := os.Getenv("SOJU_RS_BINARY")
	if binary == "" {
		t.Skip("set SOJU_RS_BINARY to a prebuilt soju-rs release binary")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	config := filepath.Join(dir, "config")
	socket := filepath.Join(dir, "admin")
	text := fmt.Sprintf("hostname soju.test\nlisten irc+insecure://%s\nlisten unix+admin://%s\ndb sqlite3 %s\nmessage-store db\n", addr, socket, filepath.Join(dir, "soju.db"))
	if err := os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	server := exec.Command(binary, "-config", config)
	server.Stdout, server.Stderr = log, log
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		server.Process.Kill()
		server.Wait()
		log.Close()
		if t.Failed() {
			data, _ := os.ReadFile(log.Name())
			t.Logf("soju-rs log:\n%s", data)
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("soju-rs admin socket did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	ctl := func(args ...string) {
		t.Helper()
		cmd := exec.Command(filepath.Join(filepath.Dir(binary), "sojuctl"), append([]string{"-config", config}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sojuctl %v: %v\n%s", args, err, out)
		}
	}
	ctl("user", "create", "-username", "alice", "-password", "test-password", "-admin")
	upstreams := []*sojuTestUpstream{newSojuTestUpstream(t), newSojuTestUpstream(t)}
	for i, u := range upstreams {
		ctl("user", "run", "alice", "network", "create", "-name", fmt.Sprintf("net%d", i), "-addr", "irc+insecure://"+u.listener.Addr().String(), "-nick", "alice")
		select {
		case <-u.ready:
		case <-time.After(10 * time.Second):
			t.Fatal("upstream registration timed out")
		}
		ctl("user", "run", "alice", "channel", "create", fmt.Sprintf("#c/net%d", i))
		select {
		case <-u.joined:
		case <-time.After(10 * time.Second):
			t.Fatal("upstream join timed out")
		}
	}
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	in, out := ChanInOut(conn)
	root := NewSession(out, SessionParams{Nickname: "alice", Username: "alice", Auth: &SASLPlain{Username: "alice", Password: "test-password"}})
	sessions := map[string]*Session{"": root}
	detached := make(map[string]bool)
	t.Cleanup(func() {
		for id, s := range sessions {
			if id != "" {
				s.Close()
			}
		}
		root.Close()
		conn.Close()
	})
	type result struct {
		network string
		event   Event
	}
	var received []result
	pong := false
	var poll func()
	pump := func(until func() bool) {
		t.Helper()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for !until() {
			select {
			case msg, ok := <-in:
				if !ok {
					t.Fatal("soju-rs disconnected")
				}
				if msg.Command == "PONG" {
					pong = true
				}
				id := msg.Tags[NetworkTag]
				if msg.Command == "CAP" {
					t.Logf("network %q: %s", id, msg.String())
				}
				if detached[id] {
					continue
				}
				s := sessions[id]
				if s == nil {
					t.Fatalf("unknown virtual network %q: %s", id, msg.String())
				}
				ev, err := s.HandleMessage(msg)
				if err != nil {
					t.Fatalf("%s: %v", msg.String(), err)
				}
				if ev != nil {
					received = append(received, result{id, ev})
				}
				if list, ok := ev.(BouncerNetworkListEvent); ok {
					for _, n := range list {
						if _, exists := sessions[n.ID]; !exists {
							v := root.NewVirtualSession(n.ID)
							if v == nil {
								t.Fatal("multiplex not negotiated")
							}
							sessions[n.ID] = v
						}
					}
				}
				if e, ok := ev.(ErrorEvent); ok && e.Severity == SeverityFail {
					t.Fatalf("soju-rs rejected command: %#v", e)
				}
			case <-tick.C:
				if poll != nil {
					poll()
				}
			case <-timer.C:
				start := len(received) - 10
				if start < 0 {
					start = 0
				}
				t.Fatalf("timed out; last events: %#v", received[start:])
			}
		}
	}
	registered := func() bool {
		count := 0
		for _, r := range received {
			if _, ok := r.event.(SelfJoinEvent); ok {
				count++
			}
		}
		if count != 2 {
			return false
		}
		for id, s := range sessions {
			if id != "" && (!s.HasCapability("draft/multiline") || !s.HasCapability("soju.im/search")) {
				return false
			}
		}
		return true
	}
	pump(registered)
	if !root.HasCapability("soju.im/unread") || !root.HasCapability("soju.im/multiplex") {
		t.Fatal("missing soju-rs capabilities")
	}
	for id, s := range sessions {
		if id != "" && !s.HasCapability("draft/multiline") {
			t.Fatal("virtual multiline capability lost")
		}
	}
	// Two upstreams reuse the same multiline reference, with a blank line and
	// concatenated UTF-8 content. The bouncer may rewrite batch IDs itself.
	for i, u := range upstreams {
		u.send("@msgid=incoming :bob!b@host BATCH +same draft/multiline #c")
		u.send("@batch=same :bob!b@host PRIVMSG #c :hello alice")
		u.send("@batch=same;draft/multiline-concat :bob!b@host PRIVMSG #c : 世界")
		u.send("@batch=same :bob!b@host PRIVMSG #c :")
		u.send(fmt.Sprintf("@batch=same :bob!b@host PRIVMSG #c :network %d", i))
		u.send(":upstream BATCH -same")
	}
	pump(func() bool {
		count := 0
		for _, r := range received {
			if m, ok := r.event.(MessageEvent); ok && strings.HasPrefix(m.Content, "hello alice 世界\n\nnetwork ") {
				count++
			}
		}
		return count == 2
	})
	// Mentions are flushed asynchronously by soju-rs. Retry snapshots until
	// both networks report their stored mention, without relying on a sleep.
	poll = func() {
		for id, s := range sessions {
			if id != "" {
				s.NewHistoryRequest("").WithLimit(1000).Targets(time.Now().Add(time.Second), time.Unix(0, 0))
			}
		}
	}
	poll()
	pump(func() bool {
		seen := make(map[string]bool)
		for _, r := range received {
			if e, ok := r.event.(HistoryTargetsEvent); ok {
				seen[r.network] = e.Unread["#c"].Unread && e.Unread["#c"].Mention
			}
		}
		count := 0
		for _, ok := range seen {
			if ok {
				count++
			}
		}
		return count == 2
	})
	poll = nil
	for id, s := range sessions {
		if id != "" {
			s.PrivMsg("#c", "outgoing\n\n世界")
			s.NewHistoryRequest("#c").Latest()
		}
	}
	pump(func() bool {
		echoes, histories := 0, 0
		for _, r := range received {
			switch e := r.event.(type) {
			case MessageEvent:
				if e.Content == "outgoing\n\n世界" && e.User == "alice" {
					echoes++
				}
			case HistoryEvent:
				if len(e.Messages) > 0 {
					histories++
				}
			}
		}
		return echoes == 2 && histories == 2
	})
	for id, s := range sessions {
		if id != "" {
			s.Search("#c", "hello")
		}
	}
	pump(func() bool {
		seen := make(map[string]bool)
		for _, r := range received {
			if e, ok := r.event.(SearchEvent); ok {
				for _, m := range e.Messages {
					if strings.HasPrefix(m.Content, "hello alice 世界\n\nnetwork ") {
						seen[r.network] = true
					}
				}
			}
		}
		return len(seen) == 2
	})
	// A read marker clears the server snapshot, and metadata remains scoped.
	stamp := time.Now().UTC().Truncate(time.Millisecond)
	for id, s := range sessions {
		if id != "" {
			s.ReadSet("#c", stamp)
			s.MutedSet("#c", true)
			s.PinnedSet("#c", true)
		}
	}
	pump(func() bool {
		reads, metadata := 0, 0
		for _, r := range received {
			if e, ok := r.event.(ReadEvent); ok && !e.Timestamp.Before(stamp) {
				reads++
			}
			if e, ok := r.event.(MetadataChangeEvent); ok && e.Muted && e.Pinned {
				metadata++
			}
		}
		return reads >= 2 && metadata >= 2
	})
	for id, s := range sessions {
		if id != "" {
			s.NewHistoryRequest("").WithLimit(1000).Targets(time.Now().Add(2*time.Second), time.Unix(0, 0))
		}
	}
	pump(func() bool {
		count := 0
		for _, r := range received {
			if e, ok := r.event.(HistoryTargetsEvent); ok {
				if _, exists := e.Targets["#c"]; exists && !e.Unread["#c"].Unread && !e.Unread["#c"].Mention {
					count++
				}
			}
		}
		return count == 2
	})
	// Detaching one network must leave the physical connection usable.
	for id, s := range sessions {
		if id != "" {
			root.Detach(id)
			detached[id] = true
			s.Close()
			delete(sessions, id)
			break
		}
	}
	root.Send("PING", "still-connected")
	pump(func() bool { return pong })
}

type sojuTestUpstream struct {
	listener      net.Listener
	mu            sync.Mutex
	conn          net.Conn
	ready, joined chan struct{}
}

func newSojuTestUpstream(t *testing.T) *sojuTestUpstream {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u := &sojuTestUpstream{listener: l, ready: make(chan struct{}), joined: make(chan struct{})}
	t.Cleanup(func() {
		l.Close()
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.conn != nil {
			u.conn.Close()
		}
	})
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		u.mu.Lock()
		u.conn = c
		u.mu.Unlock()
		defer c.Close()
		scanner := bufio.NewScanner(c)
		nick := "alice"
		var once sync.Once
		for scanner.Scan() {
			m, err := ParseMessage(scanner.Text())
			if err != nil {
				continue
			}
			switch m.Command {
			case "CAP":
				switch m.Params[0] {
				case "LS":
					u.send(":upstream CAP * LS :batch cap-notify echo-message labeled-response message-tags server-time multi-prefix draft/multiline=max-bytes=4096,max-lines=24")
				case "REQ":
					u.send(":upstream CAP * ACK :" + m.Params[1])
				case "END":
					u.send(":upstream 001 " + nick + " :Welcome")
					u.send(":upstream 004 " + nick + " upstream test io ov")
					u.send(":upstream 005 " + nick + " CHANTYPES=# CASEMAPPING=ascii PREFIX=(ov)@+ CHANMODES=b,k,l,imnt :supported")
					u.send(":upstream 422 " + nick + " :No MOTD")
					close(u.ready)
				}
			case "NICK":
				nick = m.Params[0]
			case "PING":
				u.send(":upstream PONG :" + m.Params[len(m.Params)-1])
			case "JOIN":
				u.send(":" + nick + "!u@host JOIN " + m.Params[0])
				u.send(":upstream 353 " + nick + " = " + m.Params[0] + " :@" + nick + " bob")
				u.send(":upstream 366 " + nick + " " + m.Params[0] + " :End of NAMES")
				once.Do(func() { close(u.joined) })
			case "BATCH", "PRIVMSG", "NOTICE":
				m.Prefix = ParsePrefix(nick + "!u@host")
				u.send(m.String())
			case "WHO":
				u.send(":upstream 352 " + nick + " #c u host upstream " + nick + " H :0 Alice")
				u.send(":upstream 315 " + nick + " " + m.Params[0] + " :End of WHO")
			}
		}
	}()
	return u
}

func (u *sojuTestUpstream) send(line string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn != nil {
		fmt.Fprintf(u.conn, "%s\r\n", line)
	}
}
