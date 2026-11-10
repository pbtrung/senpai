package senpai

import (
	"reflect"
	"testing"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
)

func TestBufferStoreRestoreAndClose(t *testing.T) {
	stamp := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	app := &App{}
	app.RestoreBuffers([]BufferState{
		{Network: "1", Name: "Alice", Read: stamp},
		{Network: "2", Name: "Alice", Read: stamp.Add(time.Hour)},
		{Network: "1", Name: "Bob", Read: stamp, ClosedAt: stamp.Add(time.Minute)},
		{Name: "bad\r\nPRIVMSG"},
		{Name: ""},
	})
	app.bufferStore.put(BufferState{Network: "1", Name: "ALICE", Read: stamp.Add(-time.Hour)})
	if got := app.bufferStore.get("1", "alice").Read; !got.Equal(stamp) {
		t.Fatalf("older marker replaced saved read marker: %v", got)
	}
	if app.targetClosed("1", "bob", stamp.Add(2*time.Minute)) {
		t.Fatal("new messages should reopen a closed conversation")
	}
	if !app.targetClosed("1", "BOB", stamp) || !app.targetClosed("1", "Bob", time.Time{}) {
		t.Fatal("old snapshots and pinned metadata should keep a closed conversation closed")
	}
	states := app.bufferStore.snapshot()
	if len(states) != 3 {
		t.Fatalf("saved %d buffers, want three valid distinct conversations", len(states))
	}
	restored := &App{}
	restored.RestoreBuffers(states)
	if got := restored.bufferStore.snapshot(); !reflect.DeepEqual(got, states) {
		t.Fatalf("restore changed saved state: got %#v, want %#v", got, states)
	}
}

func TestBufferStoreServerCasemapping(t *testing.T) {
	stamp := time.Now().UTC()
	for _, ascii := range []bool{false, true} {
		t.Run(map[bool]string{false: "rfc1459", true: "ascii"}[ascii], func(t *testing.T) {
			var store bufferStore
			store.put(BufferState{Name: "[Alice]", Read: stamp})
			store.put(BufferState{Name: "{alice}", Read: stamp.Add(time.Hour), ClosedAt: stamp})
			out := make(chan irc.Message, 1024)
			session := irc.NewSession(out, irc.SessionParams{Nickname: "me", Username: "me"})
			defer session.Close()
			if ascii {
				msg, _ := irc.ParseMessage(":srv 005 me CASEMAPPING=ascii :supported")
				if _, err := session.HandleMessage(msg); err != nil {
					t.Fatal(err)
				}
			}
			store.configure("", session)
			states := store.snapshot()
			if ascii {
				if len(states) != 2 {
					t.Fatal("ASCII server's distinct names were merged")
				}
			} else if len(states) != 1 || !states[0].ClosedAt.IsZero() || !states[0].Read.Equal(stamp.Add(time.Hour)) {
				t.Fatalf("RFC1459 aliases did not merge open state and latest read marker: %#v", states)
			}
		})
	}
}
