package senpai

import (
	"fmt"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

func commandDoMe(app *App, args []string) (err error) {
	netID, buffer := app.win.CurrentBuffer()
	if buffer == "" {
		netID = app.lastQueryNet
		buffer = app.lastQuery
	}
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	content := fmt.Sprintf("\x01ACTION %s\x01", args[0])
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

func commandDoMsg(app *App, args []string) (err error) {
	target := args[0]
	content := args[1]
	return commandSendMessage(app, target, content)
}

func commandDoNames(app *App, args []string) (err error) {
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if !s.IsChannel(buffer) {
		return fmt.Errorf("this is not a channel")
	}
	var sb ui.StyledStringBuilder
	sb.SetStyle(vaxis.Style{
		Foreground: app.cfg.Colors.Status,
	})
	sb.WriteString("Names: ")
	for _, name := range s.Names(buffer) {
		if name.PowerLevel != "" {
			sb.SetStyle(vaxis.Style{
				Foreground: ui.ColorGreen,
			})
			sb.WriteString(name.PowerLevel)
			sb.SetStyle(vaxis.Style{
				Foreground: app.cfg.Colors.Status,
			})
		}
		sb.WriteString(name.Name.Name)
		sb.WriteByte(' ')
	}
	body := sb.StyledString()
	// TODO remove last space
	app.win.AddLine(netID, buffer, ui.Line{
		At:   time.Now(),
		Head: ui.ColorString("--", app.cfg.Colors.Status),
		Body: body,
	})
	return nil
}

func commandDoNick(app *App, args []string) (err error) {
	nick := args[0]
	if i := strings.IndexAny(nick, " :"); i >= 0 {
		return fmt.Errorf("illegal char %q in nickname", nick[i])
	}
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	s.ChangeNick(nick)
	return
}

func commandDoMode(app *App, args []string) (err error) {
	_, target := app.win.CurrentBuffer()
	if len(args) > 0 && !strings.HasPrefix(args[0], "+") && !strings.HasPrefix(args[0], "-") {
		target = args[0]
		args = args[1:]
	}
	flags := ""
	if len(args) > 0 {
		flags = args[0]
		args = args[1:]
	}
	modeArgs := args

	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	s.ChangeMode(target, flags, modeArgs)
	return nil
}

func commandDoQuit(app *App, args []string) (err error) {
	reason := ""
	if 0 < len(args) {
		reason = args[0]
	}
	for _, session := range app.sessions {
		session.Quit(reason)
	}
	app.win.Exit()
	return nil
}

func commandDoBouncer(app *App, args []string) (err error) {
	b, err := getBouncerService(app)
	if err != nil {
		return err
	}
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	s.PrivMsg(b, args[0])
	return nil
}

func commandDoQuote(app *App, args []string) (err error) {
	if app.cfg.Transient {
		return fmt.Errorf("usage of QUOTE is disabled")
	}
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	s.SendRaw(args[0])
	return nil
}

func commandDoList(app *App, args []string) (err error) {
	if app.cfg.Transient {
		return fmt.Errorf("usage of LIST is disabled")
	}
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	var pattern string
	if len(args) > 0 {
		pattern = args[0]
	}
	s.List(pattern)
	return nil
}

func commandDoR(app *App, args []string) (err error) {
	s := app.sessions[app.lastQueryNet]
	if s == nil {
		return errOffline
	}
	s.PrivMsg(app.lastQuery, args[0])
	if !s.HasCapability("echo-message") {
		buffer, line := app.formatMessage(s, irc.MessageEvent{
			User:            s.Nick(),
			Target:          app.lastQuery,
			TargetIsChannel: s.IsChannel(app.lastQuery),
			Command:         "PRIVMSG",
			Content:         args[0],
			Time:            time.Now(),
		})
		app.win.AddLine(app.lastQueryNet, buffer, line)
	}
	return nil
}

func commandDoTopic(app *App, args []string) (err error) {
	netID, buffer := app.win.CurrentBuffer()
	var ok bool
	if len(args) == 0 {
		ok = app.printTopic(netID, buffer)
	} else {
		s := app.sessions[netID]
		if s != nil {
			s.ChangeTopic(buffer, args[0])
			ok = true
		}
	}
	if !ok {
		return errOffline
	}
	return nil
}

func commandDoWhois(app *App, args []string) (err error) {
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	var nick string
	if len(args) == 0 {
		if channel == "" || s.IsChannel(channel) {
			return fmt.Errorf("either send this command from a DM, or specify the user")
		}
		nick = channel
	} else {
		nick = args[0]
	}
	s.Whois(nick)
	return nil
}

func commandDoWhowas(app *App, args []string) (err error) {
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	var nick string
	if len(args) == 0 {
		if channel == "" || s.IsChannel(channel) {
			return fmt.Errorf("either send this command from a DM, or specify the user")
		}
		nick = channel
	} else {
		nick = args[0]
	}
	s.Whowas(nick)
	return nil
}

func commandDoInvite(app *App, args []string) (err error) {
	nick := args[0]
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if len(args) == 2 {
		channel = args[1]
	} else if channel == "" {
		return fmt.Errorf("either send this command from a channel, or specify the channel")
	}
	s.Invite(nick, channel)
	return nil
}

func commandDoKick(app *App, args []string) (err error) {
	nick := args[0]
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	// Check whether the argument after the user is a channel, to accept both:
	// - KICK user #chan you are mean
	// - KICK user you are mean
	comment := ""
	if len(args) >= 2 {
		if s.IsChannel(args[1]) {
			channel = args[1]
		} else {
			comment = args[1] + " "
		}
	}
	if channel == "" {
		return fmt.Errorf("either send this command from a channel, or specify the channel")
	}
	if len(args) == 3 {
		comment += args[2]
	}
	s.Kick(nick, channel, comment)
	return nil
}

func commandDoBan(app *App, args []string) (err error) {
	nick := args[0]
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if len(args) == 2 {
		channel = args[1]
	} else if channel == "" {
		return fmt.Errorf("either send this command from a channel, or specify the channel")
	}
	s.ChangeMode(channel, "+b", []string{nick})
	return nil
}

func commandDoUnban(app *App, args []string) (err error) {
	nick := args[0]
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if len(args) == 2 {
		channel = args[1]
	} else if channel == "" {
		return fmt.Errorf("either send this command from a channel, or specify the channel")
	}
	s.ChangeMode(channel, "-b", []string{nick})
	return nil
}

func commandDoAway(app *App, args []string) (err error) {
	reason := "Away"
	if len(args) > 0 {
		reason = args[0]
	}
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	s.Away(reason)
	return nil
}

func commandDoBack(app *App, args []string) (err error) {
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	s.Away("")
	return nil
}

func commandDoShrug(app *App, args []string) (err error) {
	_, buffer := app.win.CurrentBuffer()
	return commandSendMessage(app, buffer, `¯\_(ツ)_/¯`)
}

func commandDoTableFlip(app *App, args []string) (err error) {
	_, buffer := app.win.CurrentBuffer()
	return commandSendMessage(app, buffer, `(╯°□°)╯︵ ┻━┻`)
}

func getBouncerService(app *App) (service string, err error) {
	if app.cfg.Transient {
		return "", fmt.Errorf("usage of BOUNCER is disabled")
	}
	s := app.CurrentSession()
	if s == nil {
		return "", errOffline
	}
	b := s.BouncerService()
	if b == "" {
		return "", fmt.Errorf("no bouncer service found on this server; try using soju")
	}
	return b, nil
}
