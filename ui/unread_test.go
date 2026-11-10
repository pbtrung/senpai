package ui

import (
	"reflect"
	"testing"
	"time"
)

func TestSearchOverlayDoesNotUpdateReadMarker(t *testing.T) {
	stamp := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	bs := BufferList{list: []buffer{{netID: "1", title: "#c", lines: []Line{
		{At: stamp, Readable: true},
	}}}}
	bs.OpenOverlay()
	bs.overlay.lines = []Line{{At: stamp.Add(time.Hour), Readable: true}}
	if network, title, at := bs.UpdateRead(); network != "" || title != "" || !at.IsZero() {
		t.Fatalf("search results emitted a read marker: %q %q %v", network, title, at)
	}
	if !bs.list[0].read.IsZero() || !bs.overlay.read.IsZero() {
		t.Fatal("search results advanced a read marker")
	}
	if _, _, ok := bs.Buffer(1); ok {
		t.Fatal("search overlay appeared in the conversation list")
	}
	bs.CloseOverlay()
	if network, title, at := bs.UpdateRead(); network != "1" || title != "#c" || !at.Equal(stamp) {
		t.Fatalf("conversation read marker did not resume: %q %q %v", network, title, at)
	}
}

func TestUnreadSnapshotAndReadMarkers(t *testing.T) {
	bs := BufferList{list: []buffer{{title: "home"}, {netID: "1", title: "#c"}}, focused: true}
	latest := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	bs.SetUnread("1", "#c", latest, true, true)
	b := &bs.list[1]
	if !b.unread || b.highlights != 1 {
		t.Fatalf("snapshot not displayed: %#v", b)
	}
	bs.SetRead("1", "#c", latest.Add(-time.Second))
	if !b.unread || b.highlights != 1 {
		t.Fatal("older marker cleared unloaded unread messages")
	}
	bs.SetRead("1", "#c", latest)
	if b.unread || b.highlights != 0 {
		t.Fatal("current marker did not clear unread state")
	}
	bs.SetUnread("1", "#c", latest, true, true)
	if b.unread {
		t.Fatal("late snapshot overrode newer read marker")
	}
}

func TestMultilineLayout(t *testing.T) {
	for _, test := range []struct {
		text   string
		width  int
		breaks []int
	}{
		{"hello\nworld", 20, []int{5}},
		{"hello\n\nworld", 20, []int{5, 6}},
		{"\nhello", 20, []int{0}},
		{"hello\n", 20, []int{5}},
		{"hello\n世界", 20, []int{5}},
		{"abcdef\nghijkl", 3, []int{3, 6, 10}},
	} {
		line := Line{Body: PlainString(test.text)}
		line.computeSplitPoints(nil)
		if got := line.NewLines(nil, test.width); !reflect.DeepEqual(got, test.breaks) {
			t.Errorf("%q: got %v, want %v", test.text, got, test.breaks)
		}
	}
}

func TestHistoryDoesNotDoubleCountHighlights(t *testing.T) {
	stamp := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	bs := BufferList{ui: &UI{}, list: []buffer{{title: "home"}, {netID: "1", title: "#c", read: stamp}}, focused: true}
	bs.SetUnread("1", "#c", stamp.Add(2*time.Second), true, true)
	bs.AddLines("1", "#c", []Line{
		{At: stamp.Add(time.Second), Body: PlainString("one"), Notify: NotifyHighlight, Readable: true},
		{At: stamp.Add(2 * time.Second), Body: PlainString("two"), Notify: NotifyHighlight, Readable: true},
	}, nil)
	if got := bs.list[1].highlights; got != 2 {
		t.Fatalf("snapshot double counted: %d", got)
	}
	bs.AddLines("1", "#c", []Line{{At: stamp.Add(-time.Second), Body: PlainString("older"), Readable: true}}, nil)
	if got := bs.list[1].highlights; got != 2 {
		t.Fatalf("history reload double counted: %d", got)
	}
	bs.SetRead("1", "#c", stamp.Add(time.Second))
	if got := bs.list[1].highlights; got != 1 {
		t.Fatalf("partial read retained old highlights: %d", got)
	}
}
