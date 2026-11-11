package senpai

import (
	"time"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
)

func (app *App) handleHistoryEvents(netID string, s *irc.Session, msg irc.Message, ev irc.Event) {
	switch ev := ev.(type) {
	case irc.HistoryTargetsEvent:
		if s.HasCapability("soju.im/unread") {
			for target, last := range ev.Targets {
				state := ev.Unread[target]
				if !s.IsChannel(target) {
					if app.targetClosed(netID, target, last) {
						continue
					}
					// A mention identifies a private conversation. Notice-only
					// targets (e.g. services) should not create empty entries.
					if saved := app.bufferStore.get(netID, target); !state.Mention && saved.Name == "" {
						continue
					}
					app.addUserBuffer(netID, target, time.Time{})
				}
				app.win.SetUnread(netID, target, last, state.Unread, state.Mention)
			}
			break
		}
		type target struct {
			name string
			last time.Time
		}
		// try to fetch the history of the last opened buffer first
		targets := make([]target, 0, len(ev.Targets))
		if app.lastNetID == netID {
			if last, ok := ev.Targets[app.lastBuffer]; ok {
				targets = append(targets, target{app.lastBuffer, last})
				delete(ev.Targets, app.lastBuffer)
			}
		}
		for name, last := range ev.Targets {
			targets = append(targets, target{name, last})
		}
		for _, target := range targets {
			if s.IsChannel(target.name) {
				continue
			}
			if app.targetClosed(netID, target.name, target.last) {
				continue
			}
			// CHATHISTORY BEFORE excludes its bound, so add 1ms
			// (precision of the time tag) to include that last message.
			target.last = target.last.Add(1 * time.Millisecond)
			app.addUserBuffer(netID, target.name, target.last)
		}

	case irc.HistoryEvent:
		var linesBefore []ui.Line
		var linesAfter []ui.Line
		bk := boundKey{netID, s.Casemap(ev.Target)}
		bounds, hasBounds := app.messageBounds[bk]
		boundsNew := bounds
		for _, m := range ev.Messages {
			var line ui.Line
			switch ev := m.(type) {
			case irc.MessageEvent:
				_, line = app.formatMessage(s, ev)
			default:
				line = app.formatEvent(ev)
			}
			if line.IsZero() {
				continue
			}
			boundsNew.Update(&line)
			if _, ok := m.(irc.MessageEvent); !ok && !app.cfg.StatusEnabled {
				continue
			}
			if hasBounds {
				c := bounds.Compare(&line)
				if c < 0 {
					linesBefore = append(linesBefore, line)
				} else if c > 0 {
					linesAfter = append(linesAfter, line)
				}
			} else {
				linesBefore = append(linesBefore, line)
			}
		}
		app.win.AddLines(netID, ev.Target, linesBefore, linesAfter)

		if !boundsNew.IsZero() {
			app.messageBounds[bk] = boundsNew
		}
		if len(ev.Messages) < 10 || ev.End {
			// We're getting a non-full page, or the server told us via the draft/chathistory-end tag
			// that this is the end of available history: mark as complete to avoid indefinitely
			// fetching the history.
			// The < 10 heuristic remains for servers that don't yet emit the tag. It should ideally
			// be equal to the CHATHISTORY LIMIT, but it can be non advertised, or a full page could
			// sometimes be less than a limit (because it could be filtered). It is also not zero,
			// because bounds are inclusive, and not one, because we truncate based on the second of
			// the message (because some bouncers have a second-level resolution). Be safe and pick
			// 10 messages: less messages means that this was not a full page and we are done with
			// fetching the backlog.
			b := app.messageBounds[bk]
			b.complete = true
			app.messageBounds[bk] = b
		}

	case irc.SearchEvent:
		app.win.OpenOverlay("Press Escape to close the search results")
		lines := make([]ui.Line, 0, len(ev.Messages))
		for _, m := range ev.Messages {
			_, line := app.formatMessage(s, m)
			if line.IsZero() {
				continue
			}
			lines = append(lines, line)
		}
		app.win.AddLines("", ui.Overlay, lines, nil)

	case irc.ReadEvent:
		app.win.SetRead(netID, ev.Target, ev.Timestamp)
		state := app.bufferStore.get(netID, ev.Target)
		if state.Name != "" {
			state.Read = ev.Timestamp
			app.bufferStore.put(state)
		}
	}
}
