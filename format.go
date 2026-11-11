package senpai

import (
	"fmt"
	"strings"
	"unicode"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

type mergedEvent struct {
	oldNick        string
	nick           string
	nickCf         string
	firstConnected int // -1: offline; 1: online
	lastConnected  int // -1: offline; 1: online
	modeSet        string
	modeUnset      string
	channelMode    string
}

// formatEvent returns a formatted ui.Line for an irc.Event.
func (app *App) formatEvent(ev irc.Event) ui.Line {
	switch ev := ev.(type) {
	case irc.UserNickEvent:
		var body ui.StyledStringBuilder
		body.WriteString(fmt.Sprintf("%s\u2192%s", ev.FormerNick, ev.User))
		textStyle := vaxis.Style{
			Foreground: app.cfg.Colors.Status,
		}
		var arrowStyle vaxis.Style
		body.AddStyle(0, textStyle)
		body.AddStyle(len(ev.FormerNick), arrowStyle)
		body.AddStyle(body.Len()-len(ev.User), textStyle)
		return ui.Line{
			At:        ev.Time,
			Head:      ui.ColorString("--", app.cfg.Colors.Status),
			Body:      body.StyledString(),
			Mergeable: true,
			Data:      []irc.Event{ev},
			Readable:  true,
		}
	case irc.UserJoinEvent:
		var body ui.StyledStringBuilder
		body.Grow(len(ev.User) + 1)
		body.SetStyle(vaxis.Style{
			Foreground: ui.ColorGreen,
		})
		body.WriteByte('+')
		body.SetStyle(vaxis.Style{
			Foreground: app.cfg.Colors.Status,
		})
		body.WriteString(ev.User)
		return ui.Line{
			At:        ev.Time,
			Head:      ui.ColorString("--", app.cfg.Colors.Status),
			Body:      body.StyledString(),
			Mergeable: true,
			Data:      []irc.Event{ev},
			Readable:  true,
		}
	case irc.UserPartEvent:
		var body ui.StyledStringBuilder
		body.Grow(len(ev.User) + 1)
		body.SetStyle(vaxis.Style{
			Foreground: ui.ColorRed,
		})
		body.WriteByte('-')
		body.SetStyle(vaxis.Style{
			Foreground: app.cfg.Colors.Status,
		})
		body.WriteString(ev.User)
		return ui.Line{
			At:        ev.Time,
			Head:      ui.ColorString("--", app.cfg.Colors.Status),
			Body:      body.StyledString(),
			Mergeable: true,
			Data:      []irc.Event{ev},
			Readable:  true,
		}
	case irc.UserQuitEvent:
		var body ui.StyledStringBuilder
		body.Grow(len(ev.User) + 1)
		body.SetStyle(vaxis.Style{
			Foreground: ui.ColorRed,
		})
		body.WriteByte('-')
		body.SetStyle(vaxis.Style{
			Foreground: app.cfg.Colors.Status,
		})
		body.WriteString(ev.User)
		return ui.Line{
			At:        ev.Time,
			Head:      ui.ColorString("--", app.cfg.Colors.Status),
			Body:      body.StyledString(),
			Mergeable: true,
			Data:      []irc.Event{ev},
			Readable:  true,
		}
	case irc.TopicChangeEvent:
		topic := ui.IRCString(ev.Topic).String()
		who := ui.IRCString(ev.Who).String()
		body := fmt.Sprintf("Topic changed by %s to: %s", who, topic)
		return ui.Line{
			At:     ev.Time,
			Head:   ui.ColorString("--", app.cfg.Colors.Status),
			Notify: ui.NotifyUnread,
			Body: ui.Styled(body, vaxis.Style{
				Foreground: app.cfg.Colors.Status,
			}),
			Readable: true,
		}
	case irc.ModeChangeEvent:
		body := fmt.Sprintf("[%s]", ev.Mode)
		return ui.Line{
			At:   ev.Time,
			Head: ui.ColorString("--", app.cfg.Colors.Status),
			Body: ui.Styled(body, vaxis.Style{
				Foreground: app.cfg.Colors.Status,
			}),
			Mergeable: true,
			Data:      []irc.Event{ev},
			Readable:  true,
		}
	case *mergedEvent:
		var body ui.StyledStringBuilder
		if ev.nick != "" && ((ev.firstConnected != 0 && ev.firstConnected == ev.lastConnected) || ev.modeSet != "" || ev.modeUnset != "" || (ev.oldNick != "" && ev.oldNick != ev.nick)) {
			if ev.firstConnected != 0 && ev.firstConnected == ev.lastConnected {
				if ev.firstConnected == -1 {
					body.SetStyle(vaxis.Style{
						Foreground: ui.ColorRed,
					})
					body.WriteByte('-')
				} else {
					body.SetStyle(vaxis.Style{
						Foreground: ui.ColorGreen,
					})
					body.WriteByte('+')
				}
			}
			if ev.modeSet != "" || ev.modeUnset != "" {
				body.SetStyle(vaxis.Style{
					Foreground: app.cfg.Colors.Status,
				})
				body.WriteByte('[')
				if ev.modeSet != "" {
					body.WriteByte('+')
					body.WriteString(ev.modeSet)
				}
				if ev.modeUnset != "" {
					body.WriteByte('-')
					body.WriteString(ev.modeUnset)
				}
				body.WriteByte(']')
			}
			if ev.oldNick != "" && ev.oldNick != ev.nick {
				body.SetStyle(vaxis.Style{
					Foreground: app.cfg.Colors.Status,
				})
				body.WriteString(ev.oldNick)
				body.SetStyle(vaxis.Style{})
				body.WriteString("\u2192")
			}
			body.SetStyle(vaxis.Style{
				Foreground: app.cfg.Colors.Status,
			})
			body.WriteString(ev.nick)
		} else if ev.nick == "" && ev.channelMode != "" {
			body.SetStyle(vaxis.Style{
				Foreground: app.cfg.Colors.Status,
			})
			fmt.Fprintf(&body, "[%s]", ev.channelMode)
		} else {
			return ui.Line{}
		}
		return ui.Line{
			// Only the Body is used for merged events
			Body: body.StyledString(),
		}
	default:
		return ui.Line{}
	}
}

// formatMessage sets how a given message must be formatted.
//
// It computes three things:
// - which buffer the message must be added to,
// - the UI line.
func (app *App) formatMessage(s *irc.Session, ev irc.MessageEvent) (buffer string, line ui.Line) {
	isFromSelf := s.IsMe(ev.User)
	isToSelf := s.IsMe(ev.Target)
	isQuery := !ev.TargetIsChannel && ev.Command == "PRIVMSG"
	isNotice := ev.Command == "NOTICE"

	content := strings.TrimSuffix(ev.Content, "\x01")
	content = strings.TrimRightFunc(content, unicode.IsSpace)

	isAction := false
	if strings.HasPrefix(content, "\x01") {
		parts := strings.SplitN(content[1:], " ", 2)
		if len(parts) < 2 {
			return
		}
		switch parts[0] {
		case "ACTION":
			isAction = true
		default:
			return
		}
		content = parts[1]
	}

	styledContent := ui.IRCString(content)
	isHighlight := ev.TargetIsChannel && app.isHighlight(s, styledContent.String())

	if !ev.TargetIsChannel && (isNotice || ev.User == s.BouncerService()) {
		curNetID, curBuffer := app.win.CurrentBuffer()
		if curNetID == s.NetID() {
			buffer = curBuffer
		}
	} else if isToSelf {
		buffer = ev.User
	} else {
		buffer = ev.Target
	}

	var notification ui.NotifyType
	hlLine := ev.TargetIsChannel && isHighlight && !isFromSelf
	if isFromSelf {
		notification = ui.NotifyNone
	} else if isHighlight || isQuery {
		notification = ui.NotifyHighlight
	} else {
		notification = ui.NotifyUnread
	}

	var head ui.StyledStringBuilder
	if ev.TargetPrefix != "" {
		head.WriteStyledString(ui.ColorString(ev.TargetPrefix, ui.ColorGreen))
	}

	if isAction || isNotice {
		if head.Len() == 0 {
			head.WriteStyledString(ui.PlainString("*"))
		}
	} else {
		if head.Len() > 0 {
			head.WriteStyledString(ui.PlainString(" "))
		}
		c := app.win.IdentColor(app.cfg.Colors.Nicks, ev.User, isFromSelf)
		head.WriteStyledString(ui.ColorString(ev.User, c))
	}

	var body ui.StyledStringBuilder
	if isNotice {
		color := app.win.IdentColor(app.cfg.Colors.Nicks, ev.User, isFromSelf)
		body.SetStyle(vaxis.Style{
			Foreground: color,
		})
		body.WriteString(ev.User)
		body.SetStyle(vaxis.Style{})
		body.WriteString(": ")
		body.WriteStyledString(styledContent)
	} else if isAction {
		color := app.win.IdentColor(app.cfg.Colors.Nicks, ev.User, isFromSelf)
		body.SetStyle(vaxis.Style{
			Foreground: color,
		})
		body.WriteString(ev.User)
		body.SetStyle(vaxis.Style{})
		body.WriteString(" ")
		body.WriteStyledString(styledContent)
	} else {
		body.WriteStyledString(styledContent)
	}

	line = ui.Line{
		At:        ev.Time,
		Head:      head.StyledString(),
		Notify:    notification,
		Body:      body.StyledString(),
		Highlight: hlLine,
		Readable:  true,
	}
	return
}

func (app *App) mergeLine(former *ui.Line, addition ui.Line) {
	events := append(former.Data.([]irc.Event), addition.Data.([]irc.Event)...)
	flows := make([]*mergedEvent, 0, len(events))
	flowNick := func(nick string) *mergedEvent {
		nickCf := strings.ToLower(nick)
		for _, f := range flows {
			if f.nickCf == nickCf {
				return f
			}
		}
		return nil
	}

	for _, ev := range events {
		switch ev := ev.(type) {
		case irc.UserNickEvent:
			if f := flowNick(ev.User); f != nil {
				// Drop any existing flow on the target user, effectively replacing it
				// with this new nick. Not very "accurate", but handles disconnects/reconnects/alternate nicks
				// quietly enough.
				for i, ff := range flows {
					if f == ff {
						flows = append(flows[:i], flows[i+1:]...)
						break
					}
				}
			}
			f := flowNick(ev.FormerNick)
			if f != nil {
				f.nick = ev.User
				f.nickCf = strings.ToLower(ev.User)
			} else {
				flows = append(flows, &mergedEvent{
					oldNick: ev.FormerNick,
					nick:    ev.User,
					nickCf:  strings.ToLower(ev.User),
				})
			}
		case irc.UserJoinEvent:
			f := flowNick(ev.User)
			if f != nil {
				if f.firstConnected == 0 {
					f.firstConnected = 1
				}
				f.lastConnected = 1
				f.modeSet = ""
				f.modeUnset = ""
			} else {
				flows = append(flows, &mergedEvent{
					nick:           ev.User,
					nickCf:         strings.ToLower(ev.User),
					firstConnected: 1,
					lastConnected:  1,
				})
			}
		case irc.UserPartEvent:
			f := flowNick(ev.User)
			if f != nil {
				if f.firstConnected == 0 {
					f.firstConnected = -1
				}
				f.lastConnected = -1
				f.modeSet = ""
				f.modeUnset = ""
			} else {
				flows = append(flows, &mergedEvent{
					nick:           ev.User,
					nickCf:         strings.ToLower(ev.User),
					firstConnected: -1,
					lastConnected:  -1,
				})
			}
		case irc.UserQuitEvent:
			f := flowNick(ev.User)
			if f != nil {
				if f.firstConnected == 0 {
					f.firstConnected = -1
				}
				f.lastConnected = -1
				f.modeSet = ""
				f.modeUnset = ""
			} else {
				flows = append(flows, &mergedEvent{
					nick:           ev.User,
					nickCf:         strings.ToLower(ev.User),
					firstConnected: -1,
					lastConnected:  -1,
				})
			}
		case irc.ModeChangeEvent:
			// best-effort heuristic for guessing simple user mode changes:
			// expect "<+/-><chars> <args...>" with as many chars as args

			mode := strings.Split(ev.Mode, " ")
			modeStr := mode[0]
			modeArgs := mode[1:]
			if len(modeStr) > 0 && (modeStr[0] == '+' || modeStr[0] == '-') && len(modeArgs) == len(modeStr)-1 {
				set := modeStr[0] == '+'
				for i, nick := range modeArgs {
					f := flowNick(nick)
					if f == nil {
						f = &mergedEvent{
							nick:   nick,
							nickCf: strings.ToLower(nick),
						}
						flows = append(flows, f)
					}

					mode := string(modeStr[i+1])
					if set {
						if strings.Contains(f.modeUnset, mode) {
							f.modeUnset = strings.ReplaceAll(f.modeUnset, mode, "")
						} else if !strings.Contains(f.modeSet, mode) {
							f.modeSet += mode
						}
					} else {
						if strings.Contains(f.modeSet, mode) {
							f.modeSet = strings.ReplaceAll(f.modeSet, mode, "")
						} else if !strings.Contains(f.modeUnset, mode) {
							f.modeUnset += mode
						}
					}
				}
			} else {
				if f := flowNick(""); f != nil && f.channelMode == ev.Mode {
					// setting the same channel mode string, ignore
				} else {
					flows = append(flows, &mergedEvent{
						channelMode: ev.Mode,
					})
				}
			}
		}
	}

	newBody := new(ui.StyledStringBuilder)
	newBody.Grow(128)
	first := true
	for _, f := range flows {
		l := app.formatEvent(f)
		if l.IsZero() {
			continue
		}
		if first {
			first = false
		} else {
			newBody.WriteString("  ")
		}
		newBody.WriteStyledString(l.Body)
	}
	former.Body = newBody.StyledString()
	former.Data = events
}
