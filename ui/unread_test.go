package ui

import (
	"reflect"
	"testing"
	"time"
)

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
