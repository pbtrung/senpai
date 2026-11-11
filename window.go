package senpai

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

func (app *App) initWindow() {
	version, ok := BuildVersion()
	if !ok {
		version = "(unknown version)"
	}
	app.win.AddBuffer("", "(home)", "")
	app.win.AddLine("", "", ui.Line{
		Head: ui.PlainString("--"),
		Body: ui.PlainString(fmt.Sprintf("Welcome to senpai %v! To get started, use the Help buttons, or enter /help for a list of commands.", version)),
		At:   time.Now(),
	})
}

type statusLine struct {
	netID string
	line  ui.Line
}

func (app *App) queueStatusLine(netID string, line ui.Line) {
	if line.At.IsZero() {
		line.At = time.Now()
	}
	app.postEvent(event{
		src: "*",
		content: statusLine{
			netID: netID,
			line:  line,
		},
	})
}

func (app *App) addStatusLine(netID string, line ui.Line) {
	currentNetID, buffer := app.win.CurrentBuffer()
	if currentNetID == netID && buffer != "" {
		app.win.AddLine(netID, buffer, line)
	}
	app.win.AddLine(netID, "", line)
}

func (app *App) setStatus() {
	if app.imageLoading && app.win.ImageReady() {
		app.imageLoading = false
		app.imageOverlay = true
	}
	if app.uploadingProgress != nil {
		app.win.SetStatus(fmt.Sprintf("Uploading file (%02.1f%%)...", *app.uploadingProgress*100))
		return
	}
	if app.imageLoading {
		app.win.SetStatus("Loading image...")
		return
	}

	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return
	}
	ts := s.Typings(buffer)
	status := ""
	if 3 < len(ts) {
		status = "several people are typing..."
	} else {
		verb := " is typing..."
		if 1 < len(ts) {
			verb = " are typing..."
			status = strings.Join(ts[:len(ts)-1], ", ") + " and "
		}
		if 0 < len(ts) {
			status += ts[len(ts)-1] + verb
		}
	}
	app.win.SetStatus(status)
}

func (app *App) setBufferNumbers() {
	input := app.win.InputContent()
	if !isCommand(input) {
		app.win.FilterBuffers(false, "")
		return
	}
	cmd, arg, _ := strings.Cut(string(input[1:]), " ")
	if cmd == "" || !strings.HasPrefix("buffer", cmd) {
		app.win.FilterBuffers(false, "")
		return
	}
	if _, err := strconv.Atoi(arg); err == nil {
		// Do not filter buffers if we are passing a buffer index
		arg = ""
	}
	app.win.FilterBuffers(true, arg)
}

func (app *App) clearBufferCommand() {
	input := app.win.InputContent()
	if !isCommand(input) {
		return
	}
	cmd, _, _ := strings.Cut(string(input[1:]), " ")
	if cmd == "" || !strings.HasPrefix("buffer", cmd) {
		return
	}
	app.win.InputClear()
}

// updatePrompt changes the prompt text according to the application context.
func (app *App) updatePrompt() {
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	command := isCommand(app.win.InputContent())
	var prompt ui.StyledString
	if buffer == "" || command {
		prompt = ui.Styled(">", vaxis.Style{
			Foreground: app.cfg.Colors.Prompt,
		})
	} else if s == nil {
		prompt = ui.Styled("<offline>", vaxis.Style{
			Foreground: ui.ColorRed,
		})
	} else {
		prompt = app.win.IdentString(app.cfg.Colors.Nicks, s.Nick(), true)
	}
	app.win.SetPrompt(prompt)
}

func (app *App) printTopic(netID, buffer string) (ok bool) {
	var body string
	s := app.sessions[netID]
	if s == nil {
		return false
	}
	topic, who, at := s.Topic(buffer)
	topic = ui.IRCString(topic).String()
	if who == nil {
		body = fmt.Sprintf("Topic: %s", topic)
	} else {
		body = fmt.Sprintf("Topic (set by %s on %s): %s", who.Name, at.Local().Format("January 2 2006 at 15:04:05"), topic)
	}
	app.win.AddLine(netID, buffer, ui.Line{
		At:   time.Now(),
		Head: ui.ColorString("--", app.cfg.Colors.Status),
		Body: ui.Styled(body, vaxis.Style{
			Foreground: app.cfg.Colors.Status,
		}),
	})
	return true
}
