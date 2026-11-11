package senpai

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

func commandDoBuffer(app *App, args []string) error {
	name := args[0]
	i, err := strconv.Atoi(name)
	if err == nil {
		if app.win.JumpBufferIndex(i - 1) {
			return nil
		}
	}
	if !app.win.JumpBuffer(args[0]) {
		return fmt.Errorf("none of the buffers match %q", name)
	}

	return nil
}

func commandDoJoin(app *App, args []string) (err error) {
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	channel := args[0]
	key := ""
	if len(args) == 2 {
		key = args[1]
	}
	s.Join(channel, key)
	return nil
}

func commandDoPart(app *App, args []string) (err error) {
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	reason := ""
	if 0 < len(args) {
		if (s != nil && s.IsChannel(args[0])) || (s == nil && app.win.HasBuffer(netID, args[0])) {
			channel = args[0]
			if 1 < len(args) {
				reason = args[1]
			}
		} else {
			reason = args[0]
		}
	}

	if channel == "" {
		return fmt.Errorf("cannot part this buffer")
	}

	app.closeBuffer(netID, channel, reason)
	return nil
}

func commandDoQuery(app *App, args []string) (err error) {
	netID, _ := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	target := args[0]
	if s.IsChannel(target) {
		return fmt.Errorf("cannot query a channel, use JOIN instead")
	}
	i, _ := app.addUserBuffer(netID, target, time.Time{})
	app.win.JumpBufferIndex(i)
	if len(args) > 1 {
		if err := commandSendMessage(app, target, args[1]); err != nil {
			return err
		}
	}
	return nil
}

func commandDoMute(app *App, args []string) (err error) {
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if !s.MutedSet(buffer, true) {
		return errNotSupported
	}
	return nil
}

func commandDoUnmute(app *App, args []string) (err error) {
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if !s.MutedSet(buffer, false) {
		return errNotSupported
	}
	return nil
}

func commandDoPin(app *App, args []string) (err error) {
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if !s.PinnedSet(buffer, true) {
		return errNotSupported
	}
	return nil
}

func commandDoUnpin(app *App, args []string) (err error) {
	netID, buffer := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if !s.PinnedSet(buffer, false) {
		return errNotSupported
	}
	return nil
}

func commandDoSearch(app *App, args []string) (err error) {
	if len(args) == 0 {
		app.win.CloseOverlay()
		return nil
	}
	text := args[0]
	netID, channel := app.win.CurrentBuffer()
	s := app.sessions[netID]
	if s == nil {
		return errOffline
	}
	if !s.HasCapability("soju.im/search") {
		return errors.New("server does not support searching")
	}
	s.Search(channel, text)
	return nil
}
