package senpai

import (
	"fmt"
	"os"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/events"
	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

func isCommand(input []rune) bool {
	// Command can't start with two slashes because that's an escape for
	// a literal slash in the message
	return len(input) >= 1 && input[0] == '/' && !(len(input) >= 2 && input[1] == '/')
}

// uiLoop retrieves events from the UI and forwards them to app.events for
// handling in app.eventLoop().
func (app *App) uiLoop() {
	for ev := range app.win.Events {
		app.postEvent(event{
			src:     "*",
			content: ev,
		})
	}
}

func (app *App) handleUIEvent(ev interface{}) bool {
	// TODO: when a no-modifier no-button mouse motion event is sent, just set the mouse cursor and avoid redrawing
	// TODO: eat QuitEvent here?
	switch ev := ev.(type) {
	case vaxis.Resize:
		app.win.Resize(ev)
	case vaxis.PasteStartEvent:
		app.pasting = true
		app.pastingInputOnly = len(app.win.InputContent()) == 0
	case vaxis.PasteEndEvent:
		app.pasting = false
		if !app.pastingInputOnly {
			break
		}
		app.pastingInputOnly = false

		path := string(app.win.InputContent())
		path = strings.TrimRight(path, "\n")
		if _, err := os.Stat(path); err != nil {
			path = dropBackslash(path)
			if _, err := os.Stat(path); err != nil {
				break
			}
		}
		app.win.InputSet(fmt.Sprintf("/upload %v", path))
	case vaxis.Mouse:
		app.handleMouseEvent(ev)
	case vaxis.Key:
		app.handleKeyEvent(ev)
	case vaxis.FocusIn:
		app.win.SetFocused(true)
	case vaxis.FocusOut:
		app.win.SetFocused(false)
	case vaxis.ColorThemeUpdate:
		app.win.SetColorTheme(ev.Mode)
	case *ui.NotifyEvent:
		app.win.JumpBufferNetwork(ev.NetID, ev.Buffer)
	case *ui.ScreenshotEvent:
		if err := commandDoUpload(app, []string{ev.Path}); err != nil {
			netID, buffer := app.win.CurrentBuffer()
			app.win.AddLine(netID, buffer, ui.Line{
				At:     time.Now(),
				Head:   ui.ColorString("!!", ui.ColorRed),
				Notify: ui.NotifyUnread,
				Body:   ui.PlainSprintf("SCREENSHOT: %s", err),
			})
			break
		}
	case statusLine:
		app.addStatusLine(ev.netID, ev.line)
	case *events.EventClickNick:
		app.handleNickEvent(ev)
	case *events.EventClickLink:
		app.handleLinkEvent(ev)
	case *events.EventClickChannel:
		app.handleChannelEvent(ev)
	case *events.EventImageLoaded:
		app.win.ShowImage(ev.Image)
		if ev.Image == nil {
			app.imageLoading = false
		}
	case *events.EventFileUpload:
		if ev.Location != "" {
			app.uploadingProgress = nil
			if len(app.win.InputContent()) == 0 {
				app.win.InputSet(ev.Location)
			} else {
				netID, buffer := app.win.CurrentBuffer()
				app.win.AddLine(netID, buffer, ui.Line{
					At:   time.Now(),
					Head: ui.PlainString("--"),
					Body: ui.PlainString(fmt.Sprintf("File uploaded at: %v", ev.Location)),
				})
			}
		} else if ev.Error != "" {
			app.uploadingProgress = nil
			netID, buffer := app.win.CurrentBuffer()
			app.win.AddLine(netID, buffer, ui.Line{
				At:   time.Now(),
				Head: ui.ColorString("!!", ui.ColorRed),
				Body: ui.PlainString(fmt.Sprintf("File upload failed: %v", ev.Error)),
			})
		} else {
			app.uploadingProgress = &ev.Progress
		}
	case *events.EventSpellCheck:
		text := string(app.win.InputContent())
		typos := make([]events.TypoRange, len(ev.Typos))
		for i, t := range ev.Typos {
			typos[i] = events.TypoRange{
				Start: utf16OffsetToRuneIndex(text, t.Start),
				End:   utf16OffsetToRuneIndex(text, t.End),
			}
		}
		app.win.SetTypos(typos)
	case *events.EventOpenLink:
		host, target, netID := app.findNetworkByLink(ev.Link)
		if netID != "" {
			if cNetID, _ := app.win.CurrentBuffer(); cNetID != netID {
				app.win.JumpBufferNetwork(netID, "")
			}
			if target == "" {
				break
			}
			if !app.win.JumpBufferNetwork(netID, target) {
				if s := app.sessions[netID]; s != nil && s.IsChannel(target) {
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
		} else {
			netID, target := app.win.CurrentBuffer()
			app.win.AddLine(netID, target, ui.Line{
				At:   time.Now(),
				Head: ui.PlainString("--"),
				Body: ui.PlainString("An IRC link of a new network was opened. Enter to add and join that network."),
			})
			app.win.InputSet(fmt.Sprintf("/bouncer network create -addr %q", host))
		}
	default:
		// TODO: missing event types
	}
	return true
}

func (app *App) handleMouseEvent(ev vaxis.Mouse) {
	const memberItems = 3
	x, y := ev.Col, ev.Row
	w, h := app.win.Size()

	if app.imageOverlay && ev.Button == vaxis.MouseLeftButton {
		if ev.EventType == vaxis.EventPress {
			app.win.ShowImage(nil)
			app.imageOverlay = false
		}
		return
	}

	if ev.Button == vaxis.MouseLeftButton && (ev.EventType == vaxis.EventRelease || ev.EventType == vaxis.EventMotion) {
		if app.win.ChannelColClicked() {
			app.win.ResizeChannelCol(x + 1)
		} else if app.win.MemberColClicked() {
			app.win.ResizeMemberCol(w - x)
		}
	}

	if ev.EventType == vaxis.EventPress {
		if ev.Button == vaxis.MouseWheelUp {
			if x < app.win.ChannelWidth() || (app.win.ChannelWidth() == 0 && y == h-1) {
				app.win.ScrollChannelUpBy(4)
			} else if x > w-app.win.MemberWidth() && y < h-memberItems*2 {
				app.win.ScrollMemberUpBy(4)
			} else if y == 0 {
				app.win.ScrollTopicLeftBy(12)
			} else {
				app.win.ScrollUpBy(4)
			}
		}
		if ev.Button == vaxis.MouseWheelDown {
			if x < app.win.ChannelWidth() || (app.win.ChannelWidth() == 0 && y == h-1) {
				app.win.ScrollChannelDownBy(4)
			} else if x > w-app.win.MemberWidth() && y < h-memberItems*2 {
				app.win.ScrollMemberDownBy(4)
			} else if y == 0 {
				app.win.ScrollTopicRightBy(12)
			} else {
				app.win.ScrollDownBy(4)
			}
		}
		if ev.Button == vaxis.MouseLeftButton {
			if x == app.win.ChannelWidth()-1 {
				app.win.ClickChannelCol(true)
			} else if x < app.win.ChannelWidth() {
				app.win.ClickBuffer(app.win.VerticalBufferOffset(y))
			} else if app.win.ChannelWidth() == 0 && y == h-1 {
				app.win.ClickBuffer(app.win.HorizontalBufferOffset(x))
			} else if x == w-app.win.MemberWidth() {
				app.win.ClickMemberCol(true)
			} else if x > w-app.win.MemberWidth() && y >= 2 && y < h-memberItems*2 {
				app.win.ClickMember(y - 2 + app.win.MemberOffset())
			} else if x > w-app.win.MemberWidth() && y >= h-memberItems*2 && (y-(h-memberItems*2))%2 == 1 {
				netID, target := app.win.CurrentBuffer()
				var failed bool
				switch (y - (h - memberItems*2)) / 2 {
				case 0:
					muted := app.win.GetMuted(netID, target)
					if s := app.sessions[netID]; s != nil && target != "" {
						if !s.MutedSet(target, !muted) {
							failed = true
						}
					}
				case 1:
					pinned := app.win.GetPinned(netID, target)
					if s := app.sessions[netID]; s != nil && target != "" {
						if !s.PinnedSet(target, !pinned) {
							failed = true
						}
					}
				case 2:
					app.closeBuffer(netID, target, "")
				}
				if failed {
					netID, buffer := app.win.CurrentBuffer()
					app.win.AddLine(netID, buffer, ui.Line{
						At:   time.Now(),
						Head: ui.ColorString("!!", ui.ColorRed),
						Body: ui.PlainString(errNotSupported.Error()),
					})
				}
			} else {
				app.win.Click(x, y, ev)
			}
		}
		if ev.Button == vaxis.MouseMiddleButton {
			i := -1
			if x < app.win.ChannelWidth() {
				i = app.win.VerticalBufferOffset(y)
			} else if app.win.ChannelWidth() == 0 && y == h-1 {
				i = app.win.HorizontalBufferOffset(x)
			}
			netID, channel, ok := app.win.Buffer(i)
			if ok && channel != "" {
				app.closeBuffer(netID, channel, "")
			}
		}
		if ev.Button == vaxis.MouseRightButton {
			app.win.Click(x, y, ev)
		}
	}
	if ev.EventType == vaxis.EventRelease {
		if x < app.win.ChannelWidth()-1 {
			if i := app.win.VerticalBufferOffset(y); i == app.win.ClickedBuffer() {
				app.win.GoToBufferNo(i)
				app.clearBufferCommand()
				app.spellCheck()
			}
		} else if app.win.ChannelWidth() == 0 && y == h-1 {
			if i := app.win.HorizontalBufferOffset(x); i >= 0 && i == app.win.ClickedBuffer() {
				app.win.GoToBufferNo(i)
				app.clearBufferCommand()
				app.spellCheck()
			}
		} else if x > w-app.win.MemberWidth() && y < h-memberItems*2 {
			if i := y - 2 + app.win.MemberOffset(); i >= 0 && i == app.win.ClickedMember() {
				netID, target := app.win.CurrentBuffer()
				if target == "" {
					switch y {
					case 2:
						if _, err := getBouncerService(app); err != nil {
							app.win.AddLine(netID, target, ui.Line{
								At:   time.Now(),
								Head: ui.ColorString("--", ui.ColorRed),
								Body: ui.PlainSprintf("Adding networks is not available: %v", err),
							})
						} else {
							app.win.AddLine(netID, target, ui.Line{
								At:   time.Now(),
								Head: ui.PlainString("--"),
								Body: ui.PlainString("To join a network/server, use /bouncer network create -addr <address> [-name <name>]"),
							})
							app.win.AddLine(netID, target, ui.Line{
								At:   time.Now(),
								Head: ui.PlainString("--"),
								Body: ui.PlainString("For details, see /bouncer help network create"),
							})
							app.win.InputSet("/bouncer network create -addr ")
						}
					case 4:
						app.win.AddLine(netID, target, ui.Line{
							At:   time.Now(),
							Head: ui.PlainString("--"),
							Body: ui.PlainString("To join a channel, use /join <#channel> [<password>]"),
						})
						app.win.InputSet("/join ")
					case 6:
						app.win.AddLine(netID, target, ui.Line{
							At:   time.Now(),
							Head: ui.PlainString("--"),
							Body: ui.PlainString("To message a user, use /query <user> [<message>]"),
						})
						app.win.InputSet("/query ")
					}
				} else if s := app.sessions[netID]; s != nil {
					members := s.Names(target)
					if i < len(members) {
						buffer := members[i].Name.Name
						i, _ := app.addUserBuffer(netID, buffer, time.Time{})
						app.win.JumpBufferIndex(i)
					}
				}
			}
		}
		app.win.ClickBuffer(-1)
		app.win.ClickMember(-1)
		app.win.ClickChannelCol(false)
		app.win.ClickMemberCol(false)
	}
	if x == app.win.ChannelWidth()-1 || x == w-app.win.MemberWidth() {
		app.win.SetMouseShape(vaxis.MouseShapeResizeHorizontal)
	} else if x < app.win.ChannelWidth()-1 || x > w-app.win.MemberWidth() || app.win.HasEvent(x, y) {
		app.win.SetMouseShape(vaxis.MouseShapeClickable)
	} else {
		app.win.SetMouseShape(vaxis.MouseShapeDefault)
	}
}

func (app *App) handleNickEvent(ev *events.EventClickNick) {
	s := app.sessions[ev.NetID]
	if s == nil {
		return
	}
	i, _ := app.addUserBuffer(ev.NetID, ev.Nick, time.Time{})
	app.win.JumpBufferIndex(i)
}

func (app *App) handleChannelEvent(ev *events.EventClickChannel) {
	s := app.sessions[ev.NetID]
	if s == nil {
		return
	}
	if !app.win.JumpBufferNetwork(ev.NetID, ev.Channel) {
		s.Join(ev.Channel, "")
	}
}

// typing sends typing notifications to the IRC server according to the user
// input.
func (app *App) typing() {
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil || !app.cfg.Typings {
		return
	}
	if buffer == "" {
		return
	}
	input := app.win.InputContent()
	if len(input) == 0 {
		s.TypingStop(buffer)
	} else if !isCommand(input) {
		s.Typing(buffer)
	}
}
