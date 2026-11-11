package ui

import (
	"fmt"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/events"
	"go.rockorager.dev/vaxis"
)

func (ui *UI) Buffer(i int) (netID, title string, ok bool) {
	return ui.bs.Buffer(i)
}

func (ui *UI) CurrentBuffer() (netID, title string) {
	return ui.bs.Current()
}

func (ui *UI) NextBuffer() {
	ui.bs.Next()
	ui.memberOffset = 0
}

func (ui *UI) PreviousBuffer() {
	ui.bs.Previous()
	ui.memberOffset = 0
}

func (ui *UI) NextUnreadBuffer() {
	ui.bs.NextUnread()
	ui.memberOffset = 0
}

func (ui *UI) PreviousUnreadBuffer() {
	ui.bs.PreviousUnread()
	ui.memberOffset = 0
}

func (ui *UI) ClickedBuffer() int {
	return ui.bs.clicked
}

func (ui *UI) ClickBuffer(i int) {
	ui.bs.clicked = i
}

func (ui *UI) ClickChannelCol(v bool) {
	ui.channelColClicked = v
}

func (ui *UI) ChannelColClicked() bool {
	return ui.channelColClicked
}

func (ui *UI) ResizeChannelCol(x int) {
	if x < 6 {
		x = 6
	} else if x > 24 {
		x = 24
	}
	if ui.channelWidth == x {
		return
	}
	ui.channelWidth = x
	ui.Relayout()
}

func (ui *UI) ClickMemberCol(v bool) {
	ui.memberColClicked = v
}

func (ui *UI) MemberColClicked() bool {
	return ui.memberColClicked
}

func (ui *UI) ResizeMemberCol(x int) {
	if x < 6 {
		x = 6
	} else if x > 24 {
		x = 24
	}
	if ui.memberWidth == x {
		return
	}
	ui.memberWidth = x
	ui.Relayout()
}

func (ui *UI) GoToBufferNo(i int) {
	if ui.bs.To(i) {
		ui.memberOffset = 0
		ui.ScrollToBuffer()
	}
}

func (ui *UI) FilterBuffers(enable bool, query string) {
	ui.bs.FilterBuffers(enable, query)
}

func (ui *UI) ClickedMember() int {
	return ui.memberClicked
}

func (ui *UI) ClickMember(i int) {
	ui.memberClicked = i
}

func (ui *UI) Click(x, y int, event vaxis.Mouse) {
	for _, ev := range ui.clickEvents {
		if x >= ev.xb && x < ev.xe && y == ev.y {
			e := ev.event
			e.(events.EventClickSetEvent).SetEvent(event)
			ui.Events <- e
			break
		}
	}
}

func (ui *UI) HasEvent(x, y int) bool {
	for _, ev := range ui.clickEvents {
		if x >= ev.xb && x < ev.xe && y == ev.y {
			return true
		}
	}
	return false
}

func (ui *UI) ScrollUp() {
	ui.bs.ScrollUp(ui.bs.tlHeight / 2)
}

func (ui *UI) ScrollDown() {
	ui.bs.ScrollDown(ui.bs.tlHeight / 2)
}

func (ui *UI) ScrollUpBy(n int) {
	ui.bs.ScrollUp(n)
}

func (ui *UI) ScrollDownBy(n int) {
	ui.bs.ScrollDown(n)
}

func (ui *UI) ScrollUpHighlight() bool {
	return ui.bs.ScrollUpHighlight()
}

func (ui *UI) ScrollDownHighlight() bool {
	return ui.bs.ScrollDownHighlight()
}

func (ui *UI) ScrollChannelUpBy(n int) {
	ui.channelOffset -= n
	if ui.channelOffset < 0 {
		ui.channelOffset = 0
	}
}

func (ui *UI) ScrollChannelDownBy(n int) {
	ui.channelOffset += n
	if ui.channelOffset > len(ui.bs.list) {
		ui.channelOffset = len(ui.bs.list)
	}
}

func (ui *UI) HorizontalBufferOffset(x int) int {
	return ui.bs.HorizontalBufferOffset(x, ui.channelOffset)
}

func (ui *UI) VerticalBufferOffset(y int) int {
	return ui.bs.VerticalBufferOffset(y, ui.channelOffset)
}

func (ui *UI) MemberOffset() int {
	return ui.memberOffset
}

func (ui *UI) ChannelWidth() int {
	return ui.channelWidth
}

func (ui *UI) MemberWidth() int {
	return ui.memberWidth
}

func (ui *UI) ToggleChannelList() {
	if ui.channelWidth == 0 {
		ui.channelWidth = ui.config.ChanColWidth
	} else {
		ui.channelWidth = 0
	}
	ui.Relayout()
}

func (ui *UI) ToggleMemberList() {
	if ui.memberWidth == 0 {
		ui.memberWidth = ui.config.MemberColWidth
	} else {
		ui.memberWidth = 0
	}
	ui.Relayout()
}

func (ui *UI) ScrollMemberUpBy(n int) {
	ui.memberOffset -= n
	if ui.memberOffset < 0 {
		ui.memberOffset = 0
	}
}

func (ui *UI) ScrollMemberDownBy(n int) {
	ui.memberOffset += n
}

func (ui *UI) ScrollTopicLeftBy(n int) {
	ui.bs.ScrollTopicLeft(n)
}

func (ui *UI) ScrollTopicRightBy(n int) {
	ui.bs.ScrollTopicRight(n)
}

func (ui *UI) LinesAboveOffset() int {
	return ui.bs.LinesAboveOffset()
}

func (ui *UI) OpenOverlay(hint string) {
	ui.bs.OpenOverlay()
	ui.overlayHint = hint
}

func (ui *UI) CloseOverlay() {
	ui.bs.CloseOverlay()
}

func (ui *UI) HasOverlay() bool {
	return ui.bs.HasOverlay()
}

func (ui *UI) AddBuffer(netID, netName, title string) (i int, added bool) {
	i, added = ui.bs.Add(netID, netName, title)
	if added {
		ui.ScrollToBuffer()
	}
	return
}

func (ui *UI) RemoveBuffer(netID, title string) {
	_ = ui.bs.Remove(netID, title)
	ui.memberOffset = 0
}

func (ui *UI) RemoveNetworkBuffers(netID string) {
	ui.bs.RemoveNetwork(netID)
	ui.memberOffset = 0
}

func (ui *UI) AddLine(netID, buffer string, line Line) {
	ui.bs.AddLine(netID, buffer, line)

	curNetID, curBuffer := ui.bs.Current()
	_, b := ui.bs.at(netID, buffer)
	focused := ui.bs.focused && curNetID == netID && curBuffer == buffer
	if b != nil && line.Notify == NotifyHighlight && !focused {
		var header string
		if buffer != line.Head.String() {
			header = fmt.Sprintf("%s — %s", buffer, line.Head.String())
		} else {
			header = line.Head.String()
		}
		id := ui.notify(NotifyEvent{
			NetID:  netID,
			Buffer: buffer,
		}, header, line.Body.String())
		if id >= 0 {
			b.notifications = append(b.notifications, id)
		}
	}
}

func (ui *UI) AddLines(netID, buffer string, before, after []Line) {
	ui.bs.AddLines(netID, buffer, before, after)
}

func (ui *UI) BufferHistoryCapacity(netID, title string) int {
	_, b := ui.bs.at(netID, title)
	if b == nil {
		return 0
	}
	return max(0, MaxBufferLines-len(b.lines))
}

func (ui *UI) ReadMarker(netID, title string) time.Time {
	_, b := ui.bs.at(netID, title)
	if b == nil {
		return time.Time{}
	}
	return b.read
}

func (ui *UI) HasBuffer(netID, title string) bool {
	_, b := ui.bs.at(netID, title)
	return b != nil
}

func (ui *UI) JumpBuffer(sub string) bool {
	subLower := strings.ToLower(sub)
	for i, b := range ui.bs.list {
		var title string
		if b.title == "" {
			title = b.netName
		} else {
			title = b.title
		}
		if strings.Contains(strings.ToLower(title), subLower) {
			if ui.bs.To(i) {
				ui.memberOffset = 0
			}
			return true
		}
	}

	return false
}

func (ui *UI) JumpBufferIndex(i int) bool {
	if i >= 0 && i < len(ui.bs.list) {
		if ui.bs.To(i) {
			ui.memberOffset = 0
		}
		return true
	}
	return false
}

func (ui *UI) JumpBufferNetwork(netID, buffer string) bool {
	for i, b := range ui.bs.list {
		if b.netID == netID && strings.ToLower(b.title) == strings.ToLower(buffer) {
			if ui.bs.To(i) {
				ui.memberOffset = 0
			}
			return true
		}
	}
	return false
}

func (ui *UI) Focused() bool {
	return ui.bs.Focused()
}

func (ui *UI) SetFocused(focused bool) {
	ui.bs.SetFocused(focused)
}

func (ui *UI) SetTopic(netID, buffer string, topic StyledString) {
	ui.bs.SetTopic(netID, buffer, topic)
}

func (ui *UI) GetPinned(netID, buffer string) bool {
	return ui.bs.GetPinned(netID, buffer)
}

func (ui *UI) SetPinned(netID, buffer string, pinned bool) int {
	return ui.bs.SetPinned(netID, buffer, pinned)
}

func (ui *UI) GetMuted(netID, buffer string) bool {
	return ui.bs.GetMuted(netID, buffer)
}

func (ui *UI) SetMuted(netID, buffer string, muted bool) int {
	return ui.bs.SetMuted(netID, buffer, muted)
}

func (ui *UI) SetUnread(netID, buffer string, latest time.Time, unread, mention bool) {
	ui.bs.SetUnread(netID, buffer, latest, unread, mention)
}

func (ui *UI) SetRead(netID, buffer string, timestamp time.Time) {
	ui.bs.SetRead(netID, buffer, timestamp)
}

func (ui *UI) UpdateRead() (netID, buffer string, timestamp time.Time) {
	return ui.bs.UpdateRead()
}

func (ui *UI) MarkAllRead() []ReadMarker {
	return ui.bs.MarkAllRead()
}

func (ui *UI) ScrollToBuffer() {
	if ui.bs.current < ui.channelOffset {
		ui.channelOffset = ui.bs.current
		return
	}

	w, h := ui.vx.window.Size()
	var first int
	if ui.channelWidth > 0 {
		first = ui.bs.current - h + 1
	} else {
		first = ui.bs.GetLeftMost(w - ui.memberWidth)
	}
	if ui.channelOffset < first {
		ui.channelOffset = first
	}
}
