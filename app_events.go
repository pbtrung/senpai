package senpai

import (
	"fmt"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

func (app *App) handleIRCEvent(netID string, ev interface{}) {
	if ev == nil {
		if netID == "" {
			for id, child := range app.sessions {
				if child.IsVirtual() {
					child.Close()
					delete(app.sessions, id)
				}
			}
		}
		if s, ok := app.sessions[netID]; ok {
			s.Close()
			delete(app.sessions, netID)
		}
		return
	}
	if s, ok := ev.(*irc.Session); ok {
		if s, ok := app.sessions[netID]; ok {
			s.Close()
		}
		if !app.wantsNetwork(netID) {
			delete(app.sessions, netID)
			delete(app.monitor, netID)
			s.Close()
			return
		}
		app.sessions[netID] = s
		if _, ok := app.monitor[netID]; !ok {
			app.monitor[netID] = make(map[string]struct{})
		}
		return
	}
	if _, ok := ev.(irc.Typing); ok {
		// Just refresh the screen.
		return
	}

	msg, ok := ev.(irc.Message)
	if !ok {
		panic("unreachable")
	}
	s, ok := app.sessions[netID]
	if !ok {
		// Late messages may arrive after a network was detached or deleted.
		return
	}

	if msg.Command == "ERROR" && s.IsVirtual() {
		app.addStatusLine(netID, ui.Line{At: msg.TimeOrNow(), Head: ui.ColorString("!!", ui.ColorRed), Body: ui.PlainString(strings.Join(msg.Params, " "))})
		app.handleIRCEvent(netID, nil)
		return
	}

	// Mutate IRC state
	ev, err := s.HandleMessage(msg)
	if err != nil {
		app.win.AddLine(netID, "", ui.Line{
			Head:   ui.ColorString("!!", ui.ColorRed),
			Notify: ui.NotifyUnread,
			Body:   ui.PlainSprintf("Received corrupt message %q: %s", msg.String(), err),
		})
		return
	}
	t := msg.TimeOrNow()
	if t.After(app.lastMessageTime) {
		app.lastMessageTime = t
	}

	if cs, ok := app.pendingCompletions[netID]; ok {
		now := time.Now()
		for i := 0; i < len(cs); i++ {
			c := &cs[i]
			var r []ui.Completion
			eat := false
			if c.deadline.After(now) {
				r = c.f(ev)
				if r == nil {
					continue
				}
				eat = true
			}
			app.win.AsyncCompletions(c.id, r)
			copy(cs[i:], cs[i+1:])
			cs = cs[:len(cs)-1]
			app.pendingCompletions[netID] = cs
			i--
			if eat {
				return
			}
		}
	}

	// Mutate UI state
	switch ev := ev.(type) {
	case irc.SelfNickEvent:
		if !app.cfg.StatusEnabled {
			break
		}
		var body ui.StyledStringBuilder
		body.WriteString(fmt.Sprintf("%s\u2192%s", ev.FormerNick, s.Nick()))
		textStyle := vaxis.Style{
			Foreground: app.cfg.Colors.Status,
		}
		var arrowStyle vaxis.Style
		body.AddStyle(0, textStyle)
		body.AddStyle(len(ev.FormerNick), arrowStyle)
		body.AddStyle(body.Len()-len(s.Nick()), textStyle)
		app.addStatusLine(netID, ui.Line{
			At:        msg.TimeOrNow(),
			Head:      ui.ColorString("--", app.cfg.Colors.Status),
			Body:      body.StyledString(),
			Highlight: true,
			Readable:  true,
		})

	case irc.UserNickEvent:
		if !app.cfg.StatusEnabled {
			break
		}
		line := app.formatEvent(ev)
		for _, c := range s.ChannelsSharedWith(ev.User) {
			app.win.AddLine(netID, c, line)
		}

	case irc.SelfJoinEvent:
		i, added := app.win.AddBuffer(netID, "", ev.Channel)
		i = app.win.SetMuted(netID, ev.Channel, s.MutedGet(ev.Channel))
		i = app.win.SetPinned(netID, ev.Channel, s.PinnedGet(ev.Channel))
		if !ev.Read.IsZero() {
			app.win.SetRead(netID, ev.Channel, ev.Read)
		}
		bk := boundKey{netID, s.Casemap(ev.Channel)}
		bounds, ok := app.messageBounds[bk]
		if s.HasCapability("soju.im/unread") {
			// The target snapshot supplies unread state; load history on opening.
		} else if added || !ok {
			if t, ok := msg.Time(); ok {
				s.NewHistoryRequest(ev.Channel).
					WithLimit(500).
					Before(t)
			} else {
				s.NewHistoryRequest(ev.Channel).
					WithLimit(500).
					Latest()
			}
		} else {
			s.NewHistoryRequest(ev.Channel).
				WithLimit(1000).
				After(bounds.last)
		}
		if ev.Requested {
			app.win.JumpBufferIndex(i)
		}
		if ev.Topic != "" {
			topic := ui.IRCString(ev.Topic).ParseURLs()
			app.win.SetTopic(netID, ev.Channel, topic)
		}

		app.openBuffer(netID, ev.Channel)

	case irc.UserJoinEvent:
		if !app.cfg.StatusEnabled {
			break
		}
		line := app.formatEvent(ev)
		app.win.AddLine(netID, ev.Channel, line)

	case irc.SelfPartEvent:
		// Keep the conversation after a remote PART or KICK. Explicit closes
		// have already removed it, along with its saved open state.
		body := "You left the channel"
		if msg.Command == "KICK" {
			body = "You were kicked from the channel"
		}
		app.win.AddLine(netID, ev.Channel, ui.Line{
			At: msg.TimeOrNow(), Head: ui.PlainString("--"), Body: ui.PlainString(body),
		})

	case irc.UserPartEvent:
		if !app.cfg.StatusEnabled {
			break
		}
		line := app.formatEvent(ev)
		app.win.AddLine(netID, ev.Channel, line)

	case irc.UserQuitEvent:
		if !app.cfg.StatusEnabled {
			break
		}
		line := app.formatEvent(ev)
		for _, c := range ev.Channels {
			app.win.AddLine(netID, c, line)
		}

	case irc.TopicChangeEvent:
		line := app.formatEvent(ev)
		app.win.AddLine(netID, ev.Channel, line)
		topic := ui.IRCString(ev.Topic).ParseURLs()
		app.win.SetTopic(netID, ev.Channel, topic)

	case irc.ModeChangeEvent:
		if !app.cfg.StatusEnabled {
			break
		}
		line := app.formatEvent(ev)
		app.win.AddLine(netID, ev.Channel, line)

	case irc.InviteEvent:
		var buffer string
		var notify ui.NotifyType
		var body string
		if s.IsMe(ev.Invitee) {
			buffer = ""
			notify = ui.NotifyHighlight
			body = fmt.Sprintf("%s invited you to join %s", ev.Inviter, ev.Channel)
		} else if s.IsMe(ev.Inviter) {
			buffer = ev.Channel
			notify = ui.NotifyNone
			body = fmt.Sprintf("You invited %s to join this channel", ev.Invitee)
		} else {
			buffer = ev.Channel
			notify = ui.NotifyUnread
			body = fmt.Sprintf("%s invited %s to join this channel", ev.Inviter, ev.Invitee)
		}
		app.win.AddLine(netID, buffer, ui.Line{
			At:     msg.TimeOrNow(),
			Head:   ui.ColorString("--", app.cfg.Colors.Status),
			Notify: notify,
			Body: ui.Styled(body, vaxis.Style{
				Foreground: app.cfg.Colors.Status,
			}),
			Highlight: notify == ui.NotifyHighlight,
			Readable:  true,
		})

	case irc.MessageEvent:
		buffer, line := app.formatMessage(s, ev)
		if line.IsZero() {
			break
		}
		if buffer != "" && !s.IsChannel(buffer) {
			t, ok := msg.Time()
			if !ok {
				t = time.Time{}
			}
			app.addUserBuffer(netID, buffer, t)
		}
		app.win.AddLine(netID, buffer, line)
		if line.Notify == ui.NotifyHighlight {
			curNetID, curBuffer := app.win.CurrentBuffer()
			current := app.win.Focused() && curNetID == netID && s.Casemap(curBuffer) == s.Casemap(buffer)
			app.notifyHighlight(buffer, ev.User, line.Body.String(), current)
		}
		if !ev.TargetIsChannel && !s.IsMe(ev.User) {
			app.lastQuery = ev.User
			app.lastQueryNet = netID
		}
		bk := boundKey{netID, s.Casemap(buffer)}
		bounds := app.messageBounds[bk]
		bounds.Update(&line)
		app.messageBounds[bk] = bounds

	case irc.MetadataChangeEvent:
		app.win.SetPinned(netID, ev.Target, ev.Pinned)
		app.win.SetMuted(netID, ev.Target, ev.Muted)
		if ev.Pinned && !s.IsChannel(ev.Target) && !app.targetClosed(netID, ev.Target, time.Time{}) {
			app.addUserBuffer(netID, ev.Target, time.Time{})
		}

	case irc.ListEvent:
		for _, item := range ev {
			text := fmt.Sprintf("There are %4s users on channel %s", item.Count, item.Channel)
			if item.Topic != "" {
				text += " -- " + item.Topic
			}
			app.addStatusLine(netID, ui.Line{
				At:   msg.TimeOrNow(),
				Head: ui.ColorString("List --", app.cfg.Colors.Status),
				Body: ui.Styled(text, vaxis.Style{
					Foreground: app.cfg.Colors.Status,
				}),
			})
		}

	case irc.InfoEvent:
		var head string
		if ev.Prefix != "" {
			head = ev.Prefix + " --"
		} else {
			head = "--"
		}
		app.addStatusLine(netID, ui.Line{
			At:   msg.TimeOrNow(),
			Head: ui.ColorString(head, app.cfg.Colors.Status),
			Body: ui.Styled(ev.Message, vaxis.Style{
				Foreground: app.cfg.Colors.Status,
			}),
		})

	case irc.ErrorEvent:
		var head string
		var body string
		switch ev.Severity {
		case irc.SeverityNote:
			app.addStatusLine(netID, ui.Line{
				At:   msg.TimeOrNow(),
				Head: ui.ColorString(fmt.Sprintf("(%s) --", ev.Code), app.cfg.Colors.Status),
				Body: ui.Styled(ev.Message, vaxis.Style{
					Foreground: app.cfg.Colors.Status,
				}),
			})
			return
		case irc.SeverityFail:
			head = "--"
			body = fmt.Sprintf("Error (code %s): %s", ev.Code, ev.Message)
		case irc.SeverityWarn:
			head = "--"
			body = fmt.Sprintf("Warning (code %s): %s", ev.Code, ev.Message)
		default:
			panic("unreachable")
		}
		app.addStatusLine(netID, ui.Line{
			At:   msg.TimeOrNow(),
			Head: ui.PlainString(head),
			Body: ui.PlainString(body),
		})
	case irc.HistoryTargetsEvent, irc.HistoryEvent, irc.SearchEvent, irc.ReadEvent:
		app.handleHistoryEvents(netID, s, msg, ev)
	case irc.RegisteredEvent, irc.BouncerNetworkEvent, irc.BouncerNetworkListEvent:
		app.handleNetworkEvents(netID, s, msg, ev)
	}
}
