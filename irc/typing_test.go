package irc

import (
	"sync"
	"testing"
	"time"
)

func TestTypingExpiryAndShutdown(t *testing.T) {
	ts := NewTypings()
	target := Typing{Target: "#c", Name: "bob"}
	ts.l.Lock()
	ts.targets[target] = time.Now().Add(-7 * time.Second)
	ts.l.Unlock()
	select {
	case got := <-ts.Stops():
		if got != target {
			t.Fatalf("unexpected timeout: %#v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("typing did not expire")
	}
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				ts.Active("#c", "bob")
				ts.Done("#c", "bob")
				ts.List("#c")
			}
			ts.Close()
		}()
	}
	workers.Wait()
	ts.Close()
	select {
	case _, ok := <-ts.Stops():
		if ok {
			t.Fatal("notification after close")
		}
	case <-time.After(time.Second):
		t.Fatal("typing loop leaked after close")
	}
}
