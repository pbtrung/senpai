package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"git.sr.ht/~delthas/senpai"
)

func TestConversationListPersistence(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	states := []senpai.BufferState{
		{Network: "1", Name: "alice", Read: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)},
		{Network: "1", Name: "bob", ClosedAt: time.Date(2026, 11, 9, 13, 0, 0, 0, time.UTC)},
	}
	if got := getBuffers("one"); got != nil {
		t.Fatalf("missing list returned %#v", got)
	}
	writeBuffers(states, "one")
	if got := getBuffers("one"); !reflect.DeepEqual(got, states) {
		t.Fatalf("round trip changed state: got %#v, want %#v", got, states)
	}
	if got := getBuffers("two"); got != nil {
		t.Fatal("conversation list leaked to another configuration")
	}
	info, err := os.Stat(buffersPath("one"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("conversation list permissions are %o, want 600", info.Mode().Perm())
	}
	writeBuffers(states[:1], "one")
	if got := getBuffers("one"); !reflect.DeepEqual(got, states[:1]) {
		t.Fatal("replacing an existing list failed")
	}
	if err := os.WriteFile(buffersPath("one"), []byte("broken JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := getBuffers("one"); got != nil {
		t.Fatal("corrupt list was restored")
	}
}
