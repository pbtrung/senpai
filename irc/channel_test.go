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

func TestWriteFailureUnblocksQueuedSenders(t *testing.T) {
	client, server := net.Pipe()
	in, out := ChanInOut(client)
	server.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		for i := 0; i < 4*chanCapacity; i++ {
			out <- NewMessage("PING", "queued")
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("write failure stranded output producers")
	}
	select {
	case _, ok := <-in:
		if ok {
			t.Fatal("unexpected incoming message")
		}
	case <-time.After(time.Second):
		t.Fatal("failed connection kept input open")
	}
}

func TestTransportAcceptsLargeTaggedLines(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	in, out := ChanInOut(client)
	defer close(out)
	text := strings.Repeat("x", 65500)
	line := "@tag=" + strings.Repeat("y", 1024) + " :bob PRIVMSG #c :" + text + "\r\n"
	go func() { server.Write([]byte(line)) }()
	select {
	case msg, ok := <-in:
		if !ok || len(msg.Params) != 2 || msg.Params[1] != text {
			t.Fatal("large tagged line was dropped")
		}
	case <-time.After(time.Second):
		t.Fatal("large tagged line was not delivered")
	}
}
