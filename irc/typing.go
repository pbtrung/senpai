package irc

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Typing is an event of Name actively typing in Target.
type Typing struct {
	Target string
	Name   string
}

// Typings keeps track of typing notification timeouts.
type Typings struct {
	l       sync.Mutex
	closed  bool
	targets map[Typing]time.Time
	stops   chan Typing
	done    chan struct{}
}

// NewTypings uses one expiry loop per session instead of one sleeping goroutine
// per notification. Only that loop closes stops, so shutdown cannot race a send.
func NewTypings() *Typings {
	ts := &Typings{targets: make(map[Typing]time.Time), stops: make(chan Typing, 16), done: make(chan struct{})}
	go func() {
		defer close(ts.stops)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case now := <-tick.C:
				ts.l.Lock()
				var expired []Typing
				for target, stamp := range ts.targets {
					if now.Sub(stamp) >= 6*time.Second {
						delete(ts.targets, target)
						expired = append(expired, target)
					}
				}
				ts.l.Unlock()
				for _, target := range expired {
					select {
					case ts.stops <- target:
					case <-ts.done:
						return
					}
				}
			case <-ts.done:
				return
			}
		}
	}()
	return ts
}

func (ts *Typings) Close() {
	ts.l.Lock()
	defer ts.l.Unlock()
	if !ts.closed {
		ts.closed = true
		close(ts.done)
	}
}

// Stops is a channel that transmits typing timeouts.
func (ts *Typings) Stops() <-chan Typing {
	return ts.stops
}

// Active should be called when a user is typing to some target.
func (ts *Typings) Active(target, name string) {
	ts.l.Lock()
	defer ts.l.Unlock()
	if !ts.closed {
		ts.targets[Typing{target, name}] = time.Now()
	}
}

// Done should be called when a user is done typing to some target.
func (ts *Typings) Done(target, name string) {
	ts.l.Lock()
	delete(ts.targets, Typing{target, name})
	ts.l.Unlock()
}

func (ts *Typings) List(target string) []string {
	ts.l.Lock()
	defer ts.l.Unlock()

	var res []string
	for t := range ts.targets {
		if target == t.Target {
			res = append(res, t.Name)
		}
	}
	return res
}

type typingStamp struct {
	Last  time.Time
	Type  int
	Limit *rate.Limiter
}
