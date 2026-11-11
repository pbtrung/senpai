package senpai

import (
	"time"

	"git.sr.ht/~delthas/senpai/ui"
)

type bound struct {
	first time.Time
	last  time.Time

	firstMessage string
	lastMessage  string

	complete bool
}

// Compare returns 0 if line is within bounds, -1 if before, 1 if after.
func (b *bound) Compare(line *ui.Line) int {
	at := line.At.Truncate(time.Second)
	if at.Before(b.first) {
		return -1
	}
	if at.After(b.last) {
		return 1
	}
	if at.Equal(b.first) && line.Body.String() != b.firstMessage {
		return -1
	}
	if at.Equal(b.last) && line.Body.String() != b.lastMessage {
		return -1
	}
	return 0
}

// Update updates the bounds to include the given line.
func (b *bound) Update(line *ui.Line) {
	if line.At.IsZero() {
		return
	}
	at := line.At.Truncate(time.Second)
	if b.first.IsZero() || at.Before(b.first) {
		b.first = at
		b.firstMessage = line.Body.String()
	}
	if b.last.IsZero() || at.After(b.last) {
		b.last = at
		b.lastMessage = line.Body.String()
	}
}

// IsZero reports whether the bound is empty.
func (b *bound) IsZero() bool {
	return b.first.IsZero()
}

type boundKey struct {
	netID  string
	target string
}

// maybeRequestHistory is a wrapper around irc.Session.RequestHistory to only request
// history when needed.
func (app *App) maybeRequestHistory() {
	if app.win.HasOverlay() {
		return
	}
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return
	}
	bk := boundKey{netID, s.Casemap(buffer)}
	if app.messageBounds[bk].complete {
		return
	}
	limit := min(200, app.win.BufferHistoryCapacity(netID, buffer))
	if limit == 0 {
		return
	}
	_, h := app.win.Size()
	if l := app.win.LinesAboveOffset(); l < h*2 && buffer != "" {
		if bound, ok := app.messageBounds[bk]; ok {
			s.NewHistoryRequest(buffer).
				WithLimit(limit).
				Before(bound.first)
		} else {
			s.NewHistoryRequest(buffer).
				WithLimit(limit).
				Latest()
		}
	}
}
