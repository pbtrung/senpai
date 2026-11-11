package senpai

import (
	"fmt"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
)

func noCommand(app *App, content string) error {
	netID, buffer := app.win.CurrentBuffer()
	if buffer == "" {
		return fmt.Errorf("can't send message to this buffer")
	}
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}

	s.PrivMsg(buffer, content)
	if !s.HasCapability("echo-message") {
		buffer, line := app.formatMessage(s, irc.MessageEvent{
			User:            s.Nick(),
			Target:          buffer,
			TargetIsChannel: s.IsChannel(buffer),
			Command:         "PRIVMSG",
			Content:         content,
			Time:            time.Now(),
		})
		app.win.AddLine(netID, buffer, line)
	}

	return nil
}

// implemented from https://golang.org/src/strings/strings.go?s=8055:8085#L310
func fieldsN(s string, n int) []string {
	s = strings.TrimSpace(s)
	if s == "" || n == 0 {
		return nil
	}
	if n == 1 {
		return []string{s}
	}
	// Start of the ASCII fast path.
	var a []string
	na := 0
	fieldStart := 0
	i := 0
	// Skip spaces in front of the input.
	for i < len(s) && s[i] == ' ' {
		i++
	}
	fieldStart = i
	for i < len(s) {
		if s[i] != ' ' {
			i++
			continue
		}
		a = append(a, s[fieldStart:i])
		na++
		i++
		// Skip spaces in between fields.
		for i < len(s) && s[i] == ' ' {
			i++
		}
		fieldStart = i
		if n != maxArgsInfinite && na+1 >= n {
			a = append(a, s[fieldStart:])
			return a
		}
	}
	if fieldStart < len(s) {
		// Last field ends at EOF.
		a = append(a, s[fieldStart:])
	}
	return a
}

func parseCommand(s string) (command, args string, isCommand bool) {
	if len(s) == 0 || s[0] != '/' {
		return "", s, false
	}
	if len(s) > 1 && s[1] == '/' {
		// Input starts with two slashes.
		return "", s[1:], false
	}

	i := strings.IndexByte(s, ' ')
	if i < 0 {
		i = len(s)
	}

	return strings.ToUpper(s[1:i]), strings.TrimLeft(s[i:], " "), true
}

func commandSendMessage(app *App, target string, content string) error {
	netID, _ := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	s.PrivMsg(target, content)
	if !s.HasCapability("echo-message") {
		buffer, line := app.formatMessage(s, irc.MessageEvent{
			User:            s.Nick(),
			Target:          target,
			TargetIsChannel: s.IsChannel(target),
			Command:         "PRIVMSG",
			Content:         content,
			Time:            time.Now(),
		})
		if buffer != "" && !s.IsChannel(target) {
			app.addUserBuffer(netID, buffer, time.Time{})
		}
		app.win.AddLine(netID, buffer, line)
	}
	return nil
}

func (app *App) handleInput(buffer, content string) error {
	confirmed := content == app.lastConfirm
	app.lastConfirm = content

	if content == "" {
		return nil
	}

	cmdName, rawArgs, isCommand := parseCommand(content)
	if !isCommand {
		if _, _, command := parseCommand(strings.TrimSpace(content)); !confirmed && command {
			// " /FOO BAR"
			return fmt.Errorf("this message looks like a command; remove the spaces at the start, or press enter again to send the message as is")
		}
		return noCommand(app, rawArgs)
	}
	if cmdName == "" {
		return fmt.Errorf("lone slash at the beginning")
	}
	if strings.HasPrefix("BUFFER", cmdName) {
		cmdName = "BUFFER"
	}

	var chosenCMDName string
	var found bool
	for key := range commands {
		if !strings.HasPrefix(key, cmdName) {
			continue
		}
		if found {
			return fmt.Errorf("ambiguous command %q (could mean %v or %v)", cmdName, chosenCMDName, key)
		}
		chosenCMDName = key
		found = true
	}
	if !found {
		if confirmed {
			if s := app.CurrentSession(); s != nil {
				if rawArgs != "" {
					s.SendRaw(fmt.Sprintf("%s %s", cmdName, rawArgs))
				} else {
					s.SendRaw(cmdName)
				}
				return nil
			} else {
				return errOffline
			}
		} else {
			return fmt.Errorf("the senpai command %q does not exist; press enter again to pass the command as is to the server", cmdName)
		}
	}

	cmd := commands[chosenCMDName]

	var args []string
	if rawArgs != "" && cmd.MaxArgs != 0 {
		args = fieldsN(rawArgs, cmd.MaxArgs)
	}

	if len(args) < cmd.MinArgs {
		return fmt.Errorf("usage: %s %s", chosenCMDName, cmd.Usage)
	}
	if buffer == "" && !cmd.AllowHome {
		return fmt.Errorf("command %s cannot be executed from a server buffer", chosenCMDName)
	}

	if cmd.Handle != nil {
		return cmd.Handle(app, args)
	} else {
		if s := app.CurrentSession(); s != nil {
			if rawArgs != "" {
				s.Send(cmdName, args...)
			} else {
				s.Send(cmdName)
			}
			return nil
		} else {
			return errOffline
		}
	}
}
