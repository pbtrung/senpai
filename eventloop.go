package senpai

import (
	"fmt"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
)

type event struct {
	src     string // "*" if UI, netID otherwise
	content interface{}
}

// eventLoop retrieves events (in batches) from the event channel and handle
// them, then draws the interface after each batch is handled.
func (app *App) eventLoop() {
	for !app.win.ShouldExit() {
		var ev event
		select {
		case <-app.stop:
			return
		case ev = <-app.events:
		}
		if !app.handleEvent(ev) {
			return
		}
		deadline := time.NewTimer(200 * time.Millisecond)
	outer:
		for {
			select {
			case <-app.stop:
				deadline.Stop()
				return
			case <-deadline.C:
				break outer
			case ev := <-app.events:
				if !app.handleEvent(ev) {
					deadline.Stop()
					return
				}
			default:
				if !deadline.Stop() {
					<-deadline.C
				}
				break outer
			}
		}

		if !app.pasting {
			if app.win.Focused() {
				if netID, buffer, timestamp := app.win.UpdateRead(); buffer != "" {
					s := app.sessions[netID]
					if s != nil {
						s.ReadSet(buffer, timestamp)
					}
					state := app.bufferStore.get(netID, buffer)
					state.Network, state.Name, state.Read = netID, buffer, timestamp
					app.bufferStore.put(state)
				}
			}
			app.maybeRequestHistory()
			app.setStatus()
			app.updatePrompt()
			app.setBufferNumbers()
			var currentMembers []irc.Member
			netID, buffer := app.win.CurrentBuffer()
			s := app.sessions[netID]
			if s != nil && buffer != "" {
				currentMembers = s.Names(buffer)
			}
			app.win.Draw(currentMembers)
			var title strings.Builder
			if higlights := app.win.Highlights(); higlights > 0 {
				fmt.Fprintf(&title, "(%d) ", higlights)
			}
			if netID != "" && buffer != "" {
				fmt.Fprintf(&title, "%s - ", buffer)
			}
			title.WriteString("senpai")
			app.win.SetTitle(title.String())
		}
	}
}

func (app *App) postEvent(ev event) bool {
	app.eventMu.RLock()
	defer app.eventMu.RUnlock()
	if app.closing.Load() {
		return false
	}
	select {
	case app.events <- ev:
		return true
	case <-app.stop:
		return false
	}
}

func (app *App) handleEvent(ev event) bool {
	if ev.src == "*" {
		if ev.content == nil {
			return false
		}
		if !app.handleUIEvent(ev.content) {
			return false
		}
	} else {
		app.handleIRCEvent(ev.src, ev.content)
	}
	return true
}
