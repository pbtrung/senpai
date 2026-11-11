package senpai

import (
	"fmt"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
)

func (app *App) handleNetworkEvents(netID string, s *irc.Session, msg irc.Message, ev irc.Event) {
	switch ev := ev.(type) {
	case irc.RegisteredEvent:
		channels := append(app.restoreNetworkBuffers(netID, s), app.cfg.Channels...)
		joined := make(map[string]bool)
		for _, channel := range channels {
			key := s.Casemap(channel)
			if joined[key] {
				continue
			}
			joined[key] = true
			// TODO: group JOIN messages
			// TODO: support autojoining channels with keys
			s.Join(channel, "")
		}
		if s.HasCapability("soju.im/unread") {
			s.NewHistoryRequest("").WithLimit(1000).Targets(msg.TimeOrNow(), time.Unix(0, 0))
		} else {
			s.NewHistoryRequest("").WithLimit(1000).Targets(app.lastCloseTime, msg.TimeOrNow())
		}
		body := "Connected to the server"
		if s.Nick() != app.cfg.Nick {
			body = fmt.Sprintf("Connected to the server as %s", s.Nick())
		}
		app.addStatusLine(netID, ui.Line{
			At:   msg.TimeOrNow(),
			Head: ui.PlainString("--"),
			Body: ui.PlainString(body),
		})
		if !app.shownBouncerNotice && !s.IsBouncer() {
			app.shownBouncerNotice = true
			for _, line := range []string{
				"senpai appears to be directly connected to an IRC server, rather than to an \x02IRC bouncer\x02. This is supported, but provides a limited IRC experience.",
				"In order to connect to multiple networks, keep message history, search through your messages, and upload files, use an \x02IRC bouncer\x02 and point senpai to the bouncer.",
				"Most senpai users use senpai with the IRC bouncer software \x02soju\x02.",
				"* You can self-host \x02soju\x02 yourself (it is free and open-source): https://soju.im/",
				"* You can also use a commercial hosted bouncer (uses \x02soju\x02 underneath), endorsed by senpai: \x02https://irctoday.com/\x02",
			} {
				app.addStatusLine(netID, ui.Line{
					At:   msg.TimeOrNow(),
					Head: ui.PlainString("Bouncer --"),
					Body: ui.IRCString(line),
				})
			}
		}
		for target := range app.monitor[s.NetID()] {
			// TODO: batch MONITOR +
			s.MonitorAdd(target)
		}

		if netID == "" || app.cfg.OpenLink == "" {
			break
		}
		_, target, foundNetID := app.findNetworkByLink(app.cfg.OpenLink)
		if netID != foundNetID {
			break
		}
		app.cfg.OpenLink = ""
		app.lastNetID = ""
		app.lastBuffer = ""
		if cNetID, _ := app.win.CurrentBuffer(); cNetID != netID {
			app.win.JumpBufferNetwork(netID, "")
		}
		if target == "" {
			break
		}
		if !app.win.JumpBufferNetwork(netID, target) {
			if s.IsChannel(target) {
				cNetID, cTarget := app.win.CurrentBuffer()
				app.win.AddLine(cNetID, cTarget, ui.Line{
					At:   time.Now(),
					Head: ui.PlainString("--"),
					Body: ui.PlainString("An IRC link of a new channel was opened. Enter to add and join that channel."),
				})
				app.win.InputSet(fmt.Sprintf("/join %v", target))
			} else {
				i, _ := app.addUserBuffer(netID, target, time.Time{})
				app.win.JumpBufferIndex(i)
			}
		}

	case irc.BouncerNetworkEvent:
		if !ev.Delete {
			app.networkLock.Lock()
			if _, ok := app.networks[ev.ID]; !ok {
				app.networks[ev.ID] = ev.Attrs
			} else {
				for k, v := range ev.Attrs {
					app.networks[ev.ID][k] = v
				}
			}
			app.networkLock.Unlock()
			_, added := app.win.AddBuffer(ev.ID, ev.Attrs["name"], "")
			if added || (s.HasCapability("soju.im/multiplex") && app.sessions[ev.ID] == nil) {
				app.startNetwork(ev.ID)
			}
		} else {
			app.networkLock.Lock()
			delete(app.networks, ev.ID)
			app.networkLock.Unlock()
			// if a session was already opened, close it now.
			// otherwise, we'll close it when it sends a new session event.
			if s, ok := app.sessions[ev.ID]; ok {
				if s.IsVirtual() {
					app.sessions[""].Detach(ev.ID)
				}
				s.Close()
				delete(app.sessions, ev.ID)
				delete(app.monitor, ev.ID)
			}
			app.win.RemoveNetworkBuffers(ev.ID)
			delete(app.bufferStore, ev.ID)
		}

	case irc.BouncerNetworkListEvent:
		for _, ev := range ev {
			app.networkLock.Lock()
			if _, ok := app.networks[ev.ID]; !ok {
				app.networks[ev.ID] = ev.Attrs
			} else {
				for k, v := range ev.Attrs {
					app.networks[ev.ID][k] = v
				}
			}
			app.networkLock.Unlock()
			_, added := app.win.AddBuffer(ev.ID, ev.Attrs["name"], "")
			if added || (s.HasCapability("soju.im/multiplex") && app.sessions[ev.ID] == nil) {
				app.startNetwork(ev.ID)
			}
		}

		if app.cfg.OpenLink == "" {
			break
		}
		host, _, foundNetID := app.findNetworkByLink(app.cfg.OpenLink)
		if foundNetID != "" {
			// We will handle the link in the session for that network.
			break
		}
		app.cfg.OpenLink = ""
		netID, target := app.win.CurrentBuffer()
		app.win.AddLine(netID, target, ui.Line{
			At:   time.Now(),
			Head: ui.PlainString("--"),
			Body: ui.PlainString("An IRC link of a new network was opened. Enter to add and join that network."),
		})
		app.win.InputSet(fmt.Sprintf("/bouncer network create -addr %q", host))
	}
}
