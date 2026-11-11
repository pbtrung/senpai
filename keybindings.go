package senpai

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

type keyMatch struct {
	keycode rune
	mods    vaxis.ModifierMask
}

func (app *App) handleAction(action string, args ...string) {
	switch action {
	case "quit":
		if app.win.InputClear() {
			app.typing()
			app.spellCheck()
		} else {
			app.win.InputSet("/quit")
		}
	case "set-editor":
		if len(app.win.InputContent()) == 0 {
			app.win.InputSet(strings.Join(args, " "))
		}
	case "cursor-start":
		app.win.InputHome()
	case "cursor-end":
		app.win.InputEnd()
	case "redraw":
		app.win.Relayout()
	case "scroll-up":
		app.win.ScrollUp()
	case "scroll-down":
		app.win.ScrollDown()
	case "buffer-next":
		app.win.NextBuffer()
		app.win.ScrollToBuffer()
		app.spellCheck()
	case "buffer-previous":
		app.win.PreviousBuffer()
		app.win.ScrollToBuffer()
		app.spellCheck()
	case "buffer-next-unread":
		app.win.NextUnreadBuffer()
		app.win.ScrollToBuffer()
		app.spellCheck()
	case "buffer-previous-unread":
		app.win.PreviousUnreadBuffer()
		app.win.ScrollToBuffer()
		app.spellCheck()
	case "buffer-mark-all-read":
		app.markAllRead()
	case "cursor-right-word":
		app.win.InputRightWord()
	case "cursor-left-word":
		app.win.InputLeftWord()
	case "cursor-right":
		app.win.InputRight()
	case "cursor-left":
		app.win.InputLeft()
	case "cursor-up":
		app.win.InputUp()
	case "cursor-down":
		app.win.InputDown()
	case "cursor-delete-previous-word":
		if app.win.InputDeleteWord() {
			app.typing()
			app.spellCheck()
		}
	case "cursor-delete-next-word":
		if app.win.InputDeleteNextWord() {
			app.typing()
			app.spellCheck()
		}
	case "cursor-delete-previous":
		if app.win.InputBackspace() {
			app.typing()
			app.spellCheck()
		}
	case "cursor-delete-next":
		if app.win.InputDelete() {
			app.typing()
			app.spellCheck()
		}
	case "cursor-delete-before":
		if app.win.InputDeleteBefore() {
			app.typing()
			app.spellCheck()
		}
	case "cursor-delete-after":
		if app.win.InputDeleteAfter() {
			app.typing()
			app.spellCheck()
		}
	case "search-editor":
		app.win.InputBackSearch()
	case "auto-complete":
		if app.win.InputAutoComplete() {
			app.typing()
			app.spellCheck()
		}
	case "close-overlay":
		app.win.CloseOverlay()
	case "toggle-channel-list":
		app.win.ToggleChannelList()
	case "toggle-member-list":
		app.win.ToggleMemberList()
	case "send":
		if !app.win.InputEnter() {
			netID, buffer := app.win.CurrentBuffer()
			input := string(app.win.InputContent())
			var err error
			parts := strings.Split(input, "\n")
			if session := app.sessions[netID]; session != nil && session.HasCapability("draft/multiline") && buffer != "" && !isCommand([]rune(input)) {
				parts = []string{input}
			}
			for _, part := range parts {
				if err = app.handleInput(buffer, part); err != nil {
					app.win.AddLine(netID, buffer, ui.Line{
						At:     time.Now(),
						Head:   ui.ColorString("!!", ui.ColorRed),
						Notify: ui.NotifyUnread,
						Body:   ui.PlainSprintf("%q: %s", input, err),
					})
					break
				}
			}
			if err == nil {
				app.win.InputFlush()
			}
		}
	case "scroll-next-highlight":
		app.win.ScrollDownHighlight()
	case "scroll-previous-highlight":
		app.win.ScrollUpHighlight()
	case "buffer":
		if len(args) > 0 {
			if n, err := strconv.Atoi(args[0]); err == nil && n >= 0 {
				app.win.GoToBufferNo(n)
			} else if args[0] == "last" {
				maxInt := int(^uint(0) >> 1)
				app.win.GoToBufferNo(maxInt)
			}
			app.spellCheck()
		}
	case "paste-hint":
		if !app.shownPasteHint {
			app.shownPasteHint = true
			netID, _ := app.win.CurrentBuffer()
			app.addStatusLine(netID, ui.Line{
				At:   time.Now(),
				Head: ui.PlainString("--"),
				Body: ui.PlainString("Use Control+Shift+V to paste text, or Control+Alt+V to upload clipboard content (e.g. images)"),
			})
		}
	case "none":
	default:
		netID, buffer := app.win.CurrentBuffer()
		app.win.AddLine(netID, buffer, ui.Line{
			At:     time.Now(),
			Head:   ui.ColorString("!!", ui.ColorRed),
			Notify: ui.NotifyUnread,
			Body:   ui.PlainSprintf("shortcut: action %q does not exist", action),
		})
	}
}

var defaultCommands = map[string][]string{
	"Control+c":       {"quit"},
	"Control+f":       {"set-editor", "/search "},
	"Control+k":       {"set-editor", "/buffer "},
	"Control+v":       {"paste-hint"},
	"Control+Alt+v":   {"set-editor", "/upload"},
	"Control+a":       {"cursor-start"},
	"Control+e":       {"cursor-end"},
	"Control+l":       {"redraw"},
	"Control+u":       {"scroll-up"},
	"Page_Up":         {"scroll-up"},
	"Control+d":       {"scroll-down"},
	"Page_Down":       {"scroll-down"},
	"Control+n":       {"buffer-next"},
	"Control+p":       {"buffer-previous"},
	"Alt+Right":       {"buffer-next"},
	"Shift+Right":     {"buffer-next-unread"},
	"Control+Right":   {"cursor-right-word"},
	"Right":           {"cursor-right"},
	"Alt+Left":        {"buffer-previous"},
	"Shift+Left":      {"buffer-previous-unread"},
	"Control+Left":    {"cursor-left-word"},
	"Left":            {"cursor-left"},
	"Alt+Up":          {"buffer-previous"},
	"Up":              {"cursor-up"},
	"Alt+Down":        {"buffer-next"},
	"Down":            {"cursor-down"},
	"Alt+Home":        {"buffer", "0"},
	"Home":            {"cursor-start"},
	"Alt+End":         {"buffer", "last"},
	"End":             {"cursor-end"},
	"Alt+BackSpace":   {"cursor-delete-previous-word"},
	"Alt+Delete":      {"cursor-delete-next-word"},
	"BackSpace":       {"cursor-delete-previous"},
	"Shift+BackSpace": {"cursor-delete-previous"},
	"Delete":          {"cursor-delete-next"},
	"Control+w":       {"cursor-delete-previous-word"},
	"Control+r":       {"search-editor"},
	"Tab":             {"auto-complete"},
	"Escape":          {"close-overlay"},
	"F7":              {"toggle-channel-list"},
	"F8":              {"toggle-member-list"},
	"\n":              {"send"},
	"\r":              {"send"},
	"Control+j":       {"send"},
	"KP_Enter":        {"send"},
	"Alt+a":           {"buffer-next-unread"},
	"Alt+h":           {"buffer-mark-all-read"},
	"Alt+n":           {"scroll-next-highlight"},
	"Alt+p":           {"scroll-previous-highlight"},
	"Alt+1":           {"buffer", "0"},
	"Alt+KP_1":        {"buffer", "0"},
	"Alt+2":           {"buffer", "1"},
	"Alt+KP_2":        {"buffer", "1"},
	"Alt+3":           {"buffer", "2"},
	"Alt+KP_3":        {"buffer", "2"},
	"Alt+4":           {"buffer", "3"},
	"Alt+KP_4":        {"buffer", "3"},
	"Alt+5":           {"buffer", "4"},
	"Alt+KP_5":        {"buffer", "4"},
	"Alt+6":           {"buffer", "5"},
	"Alt+KP_6":        {"buffer", "5"},
	"Alt+7":           {"buffer", "6"},
	"Alt+KP_7":        {"buffer", "6"},
	"Alt+8":           {"buffer", "7"},
	"Alt+KP_8":        {"buffer", "7"},
	"Alt+9":           {"buffer", "8"},
	"Alt+KP_9":        {"buffer", "8"},
}

func (app *App) handleKeyEvent(ev vaxis.Key) {
	switch ev.EventType {
	case vaxis.EventPress, vaxis.EventRepeat, vaxis.EventPaste:
	default:
		return
	}
	if len(ev.Text) == 1 && ev.Text[0] < ' ' {
		// Drop control characters text (sent by some terminal emulators)
		ev.Text = ""
	}
	if ev.Modifiers&(vaxis.ModCtrl|vaxis.ModAlt|vaxis.ModSuper|vaxis.ModMeta) != 0 {
		// Drop text when sent with modifiers preventing text
		ev.Text = ""
	}
	if ev.Text != "" {
		for _, r := range ev.Text {
			app.win.InputRune(r)
		}
		app.typing()
		app.spellCheck()
		return
	}

	if ev.EventType == vaxis.EventPaste {
		for _, k := range []keyMatch{
			{keycode: '\n'},
			{keycode: '\r'},
			{keycode: vaxis.KeyKeyPadEnter},
			{keycode: 'j', mods: vaxis.ModCtrl},
		} {
			for _, km := range keyMatches(ev) {
				if km == k {
					app.win.InputRune('\n')
					return
				}
			}
		}
	}

	for _, km := range keyMatches(ev) {
		if d := app.shortcuts[km]; len(d) != 0 {
			app.handleAction(d[0], d[1:]...)
			return
		}
	}
}

func keyNameMatch(name string) *keyMatch {
	parts := strings.Split(name, "+")
	mods := parts[:len(parts)-1]
	key := parts[len(parts)-1]

	var m vaxis.ModifierMask
	for _, mod := range mods {
		switch mod {
		case "Control":
			m |= vaxis.ModCtrl
		case "Shift":
			m |= vaxis.ModShift
		case "Alt":
			m |= vaxis.ModAlt
		case "Super":
			m |= vaxis.ModSuper
		default:
			return nil
		}
	}
	if r, n := utf8.DecodeRuneInString(key); n == len(key) {
		return &keyMatch{
			keycode: r,
			mods:    m,
		}
	}
	if r := ui.KeyNames[key]; r > 0 {
		return &keyMatch{
			keycode: r,
			mods:    m,
		}
	}
	return nil
}

func keyMatches(k vaxis.Key) []keyMatch {
	m := k.Modifiers
	m &^= vaxis.ModCapsLock
	m &^= vaxis.ModNumLock

	keys := []keyMatch{
		{
			keycode: k.Keycode,
			mods:    m,
		},
	}
	if m&vaxis.ModShift != 0 && k.ShiftedCode != 0 {
		// ctrl+. and user pressed ctrl+shift+; on a French keyboard
		m &^= vaxis.ModShift
		keys = append(keys, keyMatch{
			keycode: k.ShiftedCode,
			mods:    m &^ vaxis.ModShift,
		})
	}
	return keys
}
