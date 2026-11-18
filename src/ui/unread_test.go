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

func TestMarkAllRead(t *testing.T) {
	stamp := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	bs := BufferList{ui: &UI{}, focused: true, list: []buffer{
		{title: "", unread: true, highlights: 1},
		{netID: "1", title: "#loaded", unread: true, lines: []Line{
			{At: stamp, Readable: true, Notify: NotifyHighlight},
			{At: stamp.Add(time.Hour)}, // Local status output must not advance a marker.
		}},
		{netID: "2", title: "bob", muted: true},
		{netID: "3", title: "#empty", read: stamp.Add(time.Minute)},
	}}
	bs.SetUnread("2", "bob", stamp.Add(time.Second), true, true)
	bs.To(2) // Selecting unloaded history must preserve the timestamp for Alt+H.
	bs.OpenOverlay()
	bs.overlay.lines = []Line{{At: stamp.Add(2 * time.Hour), Readable: true}}
	got := bs.MarkAllRead()
	want := []ReadMarker{{"1", "#loaded", stamp}, {"2", "bob", stamp.Add(time.Second)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("markers: got %#v, want %#v", got, want)
	}
	if !bs.HasOverlay() || !bs.overlay.read.IsZero() {
		t.Fatal("mark-all-read changed search overlay")
	}
	if !bs.list[3].read.Equal(stamp.Add(time.Minute)) {
		t.Fatal("read marker moved backwards")
	}
	for _, b := range bs.list {
		if b.unread || b.highlights != 0 || !b.unreadLatest.IsZero() || b.snapshotMention {
			t.Fatalf("unread state remains in %s/%s", b.netID, b.title)
		}
	}
	if got := bs.MarkAllRead(); len(got) != 0 {
		t.Fatalf("repeated action resent markers: %#v", got)
	}
	bs.SetUnread("2", "bob", stamp.Add(time.Second), true, true)
	if bs.list[2].unread {
		t.Fatal("stale snapshot undid mark-all-read")
	}
	bs.CloseOverlay()
	bs.To(0)
	bs.SetUnread("2", "bob", stamp.Add(2*time.Second), true, true)
	if !bs.list[2].unread {
		t.Fatal("newer message was suppressed")
	}
}

func TestUpdateReadIgnoresStatusOnlyLines(t *testing.T) {
	bs := BufferList{ui: &UI{}, list: []buffer{{title: "#c", lines: []Line{
		{At: time.Now(), Body: PlainString("local status")},
	}}}}
	if _, _, at := bs.UpdateRead(); !at.IsZero() {
		t.Fatal("local status advanced a read marker")
	}
}

func TestPartialReadPreservesUnloadedSnapshot(t *testing.T) {
	stamp := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	bs := BufferList{list: []buffer{{title: "#c"}}, focused: true}
	bs.SetUnread("", "#c", stamp.Add(time.Hour), true, true)
	bs.SetRead("", "#c", stamp)
	if !bs.list[0].unreadLatest.Equal(stamp.Add(time.Hour)) {
		t.Fatal("partial read discarded unloaded unread snapshot")
	}
	markers := bs.MarkAllRead()
	if len(markers) != 1 || !markers[0].Timestamp.Equal(stamp.Add(time.Hour)) {
		t.Fatalf("missing snapshot read progress: %#v", markers)
	}
}
