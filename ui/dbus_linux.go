//go:build linux

package ui

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

var dbusConn *dbus.Conn
var dbusLock sync.Mutex

var notifications = make(map[int]*NotifyEvent)

var dbusMarkupOnce sync.Once
var dbusMarkup bool

func notifyDBus(title, content string) int {
	conn, err := dbus.SessionBus()
	if err != nil {
		return -1
	}
	var r uint32
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	dbusMarkupOnce.Do(func() {
		dbusMarkup = true
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var caps []string
		if err := obj.CallWithContext(ctx, "org.freedesktop.Notifications.GetCapabilities", 0).Store(&caps); err != nil {
			return
		}
		dbusMarkup = slices.Contains(caps, "body-markup")
	})
	if dbusMarkup {
		content = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(content)
	}
	err = obj.Call("org.freedesktop.Notifications.Notify", 0, "senpai", uint32(0), "senpai", title, content, []string{
		"default", "Open",
	}, map[string]dbus.Variant{
		"category":      dbus.MakeVariant("im.received"),
		"desktop-entry": dbus.MakeVariant("senpai"),
		"image-path":    dbus.MakeVariant("senpai"),
		"urgency":       dbus.MakeVariant(uint8(1)), // Normal
	}, int32(-1)).Store(&r)
	if err != nil {
		return -1
	}
	return int(r)
}

func (ui *UI) notify(target NotifyEvent, title, content string) int {
	if ui.config.LocalIntegrations {
		id := notifyDBus(title, content)
		if id > 0 {
			dbusLock.Lock()
			notifications[id] = &target
			dbusLock.Unlock()
			return id
		}
	}

	ui.vx.Notify(title, content)
	return -1
}

func notifyClose(id int) {
	dbusLock.Lock()
	delete(notifications, id)
	dbusLock.Unlock()
	conn, err := dbus.SessionBus()
	if err != nil {
		return
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	obj.Call("org.freedesktop.Notifications.CloseNotification", 0, uint32(id))
}

func Screenshot() error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("failed to connect to D-Bus: %v", err)
	}
	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	c := obj.Call("org.freedesktop.portal.Screenshot.Screenshot", 0, "", map[string]dbus.Variant{
		"modal":       dbus.MakeVariant(true),
		"interactive": dbus.MakeVariant(true),
	})
	if c.Err != nil {
		return fmt.Errorf("failed to take screenshot: %v", c.Err)
	}
	return nil
}

func DBusStart(callback func(any)) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/Notifications"),
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
		dbus.WithMatchSender("org.freedesktop.Notifications"),
	); err != nil {
		return
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchPathNamespace("/org/freedesktop/portal/desktop"),
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return
	}
	c := make(chan *dbus.Signal, 64)
	conn.Signal(c)
	dbusLock.Lock()
	dbusConn = conn
	dbusLock.Unlock()
	go func() {
		for v := range c {
			handleDBusSignal(v, callback)
		}
	}()
}

func handleDBusSignal(v *dbus.Signal, callback func(any)) {
	if v == nil || len(v.Body) < 1 {
		return
	}
	switch v.Name {
	case "org.freedesktop.Notifications.NotificationClosed":
		id, ok := v.Body[0].(uint32)
		if !ok {
			return
		}
		dbusLock.Lock()
		delete(notifications, int(id))
		dbusLock.Unlock()
	case "org.freedesktop.Notifications.ActionInvoked":
		id, ok := v.Body[0].(uint32)
		if !ok || len(v.Body) < 2 {
			return
		}
		action, ok := v.Body[1].(string)
		if !ok || action != "default" {
			return
		}
		dbusLock.Lock()
		target := notifications[int(id)]
		dbusLock.Unlock()
		if target != nil {
			callback(target)
		}
	case "org.freedesktop.portal.Request.Response":
		status, ok := v.Body[0].(uint32)
		if !ok || status != 0 || len(v.Body) < 2 {
			return
		}
		results, ok := v.Body[1].(map[string]dbus.Variant)
		if !ok {
			return
		}
		uri, ok := results["uri"].Value().(string)
		if !ok {
			return
		}
		if u, err := url.Parse(uri); err == nil && u.Scheme == "file" && (u.Host == "" || u.Host == "localhost") {
			callback(&ScreenshotEvent{Path: u.Path})
		}
	}
}

func DBusStop() {
	dbusLock.Lock()
	c := dbusConn
	dbusConn = nil
	dbusLock.Unlock()
	if c != nil {
		c.Close()
	}
}
