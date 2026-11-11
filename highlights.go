package senpai

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
)

func isWordBoundary(r rune) bool {
	switch r {
	case '-', '_', '|': // inspired from weechat.look.highlight_regex
		return false
	default:
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}
}

func isHighlight(text, nick string) bool {
	for {
		i := strings.Index(text, nick)
		if i < 0 {
			return false
		}

		left, _ := utf8.DecodeLastRuneInString(text[:i])
		right, _ := utf8.DecodeRuneInString(text[i+len(nick):])
		if isWordBoundary(left) && isWordBoundary(right) {
			return true
		}

		text = text[i+len(nick):]
	}
}

// isHighlight reports whether the given message content is a highlight.
func (app *App) isHighlight(s *irc.Session, content string) bool {
	contentCf := s.Casemap(content)
	if app.highlights == nil {
		return isHighlight(contentCf, s.NickCf())
	}
	for _, h := range app.highlights {
		if isHighlight(contentCf, s.Casemap(h)) {
			return true
		}
	}
	return false
}

// notifyHighlight executes the script at "on-highlight-path" according to the given
// message context.
func (app *App) notifyHighlight(buffer, nick, content string, current bool) {
	if !current && app.cfg.OnHighlightBeep {
		app.win.Beep()
	}

	if app.cfg.Transient {
		return
	}

	path := app.cfg.OnHighlightPath
	if path == "" {
		defaultHighlightPath, err := DefaultHighlightPath()
		if err != nil {
			return
		}
		path = defaultHighlightPath
	}

	netID, _ := app.win.CurrentBuffer()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// only error out if the user specified a highlight path
		// if default path unreachable, simple bail
		if app.cfg.OnHighlightPath != "" {
			body := fmt.Sprintf("Unable to find on-highlight command at path: %q", path)
			app.addStatusLine(netID, ui.Line{
				At:   time.Now(),
				Head: ui.ColorString("!!", ui.ColorRed),
				Body: ui.PlainString(body),
			})
		}
		return
	}
	here := "0"
	if current {
		here = "1"
	}
	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("BUFFER=%s", buffer),
		fmt.Sprintf("HERE=%s", here),
		fmt.Sprintf("SENDER=%s", nick),
		fmt.Sprintf("MESSAGE=%s", content),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		body := fmt.Sprintf("Failed to invoke on-highlight command at path: %v. Output: %q", err, string(output))
		app.addStatusLine(netID, ui.Line{
			At:   time.Now(),
			Head: ui.ColorString("!!", ui.ColorRed),
			Body: ui.PlainString(body),
		})
	}
}
