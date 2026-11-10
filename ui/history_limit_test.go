package ui

import (
	"fmt"
	"testing"
	"time"
)

func historyLine(i int) Line {
	return Line{At: time.Unix(int64(i), 0), Body: PlainString(fmt.Sprint(i)), Readable: true}
}

func TestLiveHistoryLimit(t *testing.T) {
	ui := &UI{}
	ui.bs = NewBufferList(ui)
	ui.bs.Add("", "home", "")
	ui.bs.Add("1", "network", "#c")
	for i := 1; i <= MaxBufferLines+100; i++ {
		ui.bs.AddLine("1", "#c", historyLine(i))
	}
	_, b := ui.bs.at("1", "#c")
	if len(b.lines) != MaxBufferLines || b.lines[0].Body.String() != "101" || b.lines[MaxBufferLines-1].Body.String() != "2100" {
		t.Fatal("live messages did not retain only the newest 2000 lines")
	}
	if got := ui.BufferHistoryCapacity("1", "#c"); got != 0 {
		t.Fatalf("full buffer has capacity %d", got)
	}
	if got := ui.BufferHistoryCapacity("", ""); got != MaxBufferLines {
		t.Fatalf("one buffer consumed another's capacity: %d", got)
	}
	for _, line := range b.lines[len(b.lines):cap(b.lines)] {
		if !line.IsZero() {
			t.Fatal("discarded message still referenced by the backing array")
		}
	}
}

func TestBatchedHistoryLimitAndUnread(t *testing.T) {
	ui := &UI{}
	bs := NewBufferList(ui)
	bs.ResizeTimeline(80, 22, 80)
	bs.Add("", "home", "")
	bs.Add("1", "network", "#c")
	before := make([]Line, MaxBufferLines+20)
	for i := range before {
		before[i] = historyLine(i + 1)
	}
	before[0].Notify = NotifyHighlight
	bs.SetRead("1", "#c", time.Unix(0, 0))
	bs.AddLines("1", "#c", before, nil)
	_, b := bs.at("1", "#c")
	if len(b.lines) != MaxBufferLines || b.lines[0].Body.String() != "21" {
		t.Fatal("history batch exceeded the message limit or kept oldest messages")
	}
	if !b.unread || b.highlights != 1 {
		t.Fatal("discarding an unread mention lost its unread indicator")
	}
	b.scrollAmt = MaxBufferLines * 2
	bs.AddLines("1", "#c", nil, []Line{historyLine(MaxBufferLines + 21)})
	if len(b.lines) != MaxBufferLines || b.scrollAmt > MaxBufferLines-bs.tlHeight {
		t.Fatal("appending history left the viewport beyond the retained messages")
	}
	bs.SetRead("1", "#c", time.Unix(1, 0))
	if b.unread || b.highlights != 0 {
		t.Fatal("read marker did not clear the discarded mention")
	}
}
