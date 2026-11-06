package irc

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

func TestMultiplexLabels(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	_, out := ChanInOut(client)
	defer close(out)
	server.SetReadDeadline(time.Now().Add(5 * time.Second))
	out <- Message{Command: labelEnableCommand}
	out <- Message{Command: labelEnableCommand, Tags: map[string]string{NetworkTag: "1"}}
	out <- Message{Command: labelEnableCommand, Tags: map[string]string{NetworkTag: "2"}}
	out <- Message{Command: labelDisableCommand, Tags: map[string]string{NetworkTag: "1"}}
	out <- NewMessage("PRIVMSG", "#c", "one").WithTag(NetworkTag, "1")
	out <- NewMessage("BATCH", "+m", "draft/multiline", "#c").WithTag(NetworkTag, "2")
	out <- NewMessage("PRIVMSG", "#c", "two").WithTag(NetworkTag, "2").WithTag("batch", "m")
	out <- NewMessage("BATCH", "-m").WithTag(NetworkTag, "2")
	out <- NewMessage("PING", "root")
	scanner := bufio.NewScanner(server)
	for i, wantLabel := range []bool{false, true, false, false, true} {
		if !scanner.Scan() {
			t.Fatalf("line %d: %v", i, scanner.Err())
		}
		m, err := ParseMessage(strings.TrimSuffix(scanner.Text(), "\r"))
		if err != nil {
			t.Fatal(err)
		}
		_, hasLabel := m.Tags["label"]
		if hasLabel != wantLabel {
			t.Fatalf("line %d: unexpected label: %s", i, m.String())
		}
	}
}
