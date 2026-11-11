package senpai

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
)

func TestExpiredCompletionsAreRemovedExactlyOnce(t *testing.T) {
	out := make(chan irc.Message, 4096)
	session := irc.NewSession(out, irc.SessionParams{Nickname: "me", Username: "me"})
	defer session.Close()
	calls := 0
	app := &App{
		win: &ui.UI{}, sessions: map[string]*irc.Session{"": session},
		pendingCompletions: map[string][]pendingCompletion{"": {
			{id: 1, deadline: time.Now().Add(-time.Minute)},
			{id: 2, deadline: time.Now().Add(-time.Minute)},
			{id: 3, deadline: time.Now().Add(time.Hour), f: func(irc.Event) []ui.Completion { calls++; return nil }},
		}},
	}
	app.handleIRCEvent("", irc.NewMessage("PING", "test"))
	if pending := app.pendingCompletions[""]; len(pending) != 1 || pending[0].id != 3 || calls != 1 {
		t.Fatalf("completion queue corrupted: %#v, callback calls %d", pending, calls)
	}
}

func TestCloseUnblocksEventProducers(t *testing.T) {
	win, _, err := ui.New(ui.Config{
		WithConsole:  bufferTestConsole{strings.NewReader("\x1b[?1;2c")},
		NickColWidth: 10, TextMaxWidth: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{win: win, events: make(chan event, 1), stop: make(chan struct{})}
	app.events <- event{}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() { app.postEvent(event{content: "pending"}) })
	}
	app.Close()
	app.Close()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown left event producers blocked")
	}
	if app.postEvent(event{}) {
		t.Fatal("accepted event after shutdown")
	}
}

type harperTestWriter struct{ messages chan []byte }

func (w harperTestWriter) Write(p []byte) (int, error) {
	w.messages <- append([]byte(nil), p...)
	return len(p), nil
}
func (w harperTestWriter) Close() error { return nil }

func TestHarperChangesDoNotWaitForConfigResponses(t *testing.T) {
	writer := harperTestWriter{messages: make(chan []byte, 8)}
	h := &harperState{stdin: writer, writes: make(chan []byte, 8), textCh: make(chan string, 1), done: make(chan struct{})}
	exited := make(chan struct{})
	go func() { h.writeLoop(); close(exited) }()
	defer func() {
		h.stop()
		select {
		case <-exited:
		case <-time.After(time.Second):
			t.Error("spell-check writer did not stop")
		}
	}()
	for _, text := range []string{"first", "second"} {
		h.sendChange(text)
		select {
		case msg := <-writer.messages:
			if !strings.Contains(string(msg), text) {
				t.Fatalf("wrong change: %s", msg)
			}
		case <-time.After(time.Second):
			t.Fatalf("text change %q stalled", text)
		}
	}
	h.stop()
	for i := 0; i < 100; i++ {
		_ = h.sendNotification("test", json.RawMessage(`{}`))
	}
}

func TestHarperRejectsInvalidFrameSizes(t *testing.T) {
	for _, header := range []string{"Content-Length: -1", "Content-Length: 4194305", "Content-Length: invalid", "Other: value"} {
		h := harperState{stdout: bufio.NewReader(strings.NewReader(header + "\r\n\r\n"))}
		if _, err := h.readMessage(); err == nil {
			t.Fatalf("accepted invalid header %q", header)
		}
	}
	h := harperState{stdout: bufio.NewReader(strings.NewReader("Content-Length: 5\r\n\r\nx"))}
	if _, err := h.readMessage(); err != io.ErrUnexpectedEOF {
		t.Fatalf("truncated frame: %v", err)
	}
}

func TestCloseLeavesRunningSessionsToEventLoop(t *testing.T) {
	win, _, err := ui.New(ui.Config{
		WithConsole:  bufferTestConsole{strings.NewReader("\x1b[?1;2c")},
		NickColWidth: 10, TextMaxWidth: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan irc.Message, 4096)
	session := irc.NewSession(out, irc.SessionParams{Nickname: "me", Username: "me"})
	app := &App{win: win, events: make(chan event, 1), stop: make(chan struct{}), running: true,
		sessions: map[string]*irc.Session{"": session}}
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; i < 100; i++ {
			session.PrivMsg("#c", "active event")
		}
	})
	app.Close()
	wg.Wait()
	before := len(out)
	session.PrivMsg("#c", "last event")
	if len(out) != before+1 {
		t.Fatal("Close interrupted a running session")
	}
	app.cleanup()
	session.PrivMsg("#c", "late event")
	if len(out) != before+1 {
		t.Fatal("cleanup left session open")
	}
}
