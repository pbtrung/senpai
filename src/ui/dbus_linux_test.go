//go:build linux

package ui

import (
	"github.com/godbus/dbus/v5"
	"testing"
)

func TestDBusSignalValidation(t *testing.T) {
	names := []string{
		"org.freedesktop.Notifications.NotificationClosed",
		"org.freedesktop.Notifications.ActionInvoked",
		"org.freedesktop.portal.Request.Response",
	}
	callback := func(any) { t.Error("malformed signal triggered callback") }
	handleDBusSignal(nil, callback)
	for _, name := range names {
		for _, body := range [][]any{nil, {"wrong"}, {uint32(0)}, {uint32(0), "wrong"}, {uint32(0), map[string]dbus.Variant{}}, {uint32(0), map[string]dbus.Variant{"uri": dbus.MakeVariant(123)}}} {
			handleDBusSignal(&dbus.Signal{Name: name, Body: body}, callback)
		}
	}
}

func TestDBusNotificationAndScreenshotSignals(t *testing.T) {
	target := &NotifyEvent{NetID: "1", Buffer: "#c"}
	notifications[42] = target
	defer delete(notifications, 42)
	var got any
	handleDBusSignal(&dbus.Signal{Name: "org.freedesktop.Notifications.ActionInvoked", Body: []any{uint32(42), "default"}}, func(ev any) { got = ev })
	if got != target {
		t.Fatalf("notification did not select conversation: %#v", got)
	}
	handleDBusSignal(&dbus.Signal{Name: "org.freedesktop.Notifications.NotificationClosed", Body: []any{uint32(42), uint32(2)}}, func(any) {})
	if _, ok := notifications[42]; ok {
		t.Fatal("closed notification was retained")
	}
	got = nil
	handleDBusSignal(&dbus.Signal{Name: "org.freedesktop.portal.Request.Response", Body: []any{uint32(0), map[string]dbus.Variant{"uri": dbus.MakeVariant("file:///tmp/screen%20shot.png")}}}, func(ev any) { got = ev })
	if ev, ok := got.(*ScreenshotEvent); !ok || ev.Path != "/tmp/screen shot.png" {
		t.Fatalf("screenshot result not decoded: %#v", got)
	}
}
