package ui

import (
	"sort"
	"strings"
	"time"
)

const Overlay = "/overlay"

// MaxBufferLines bounds the in-memory history of each conversation.
const MaxBufferLines = 2000

const (
	NotifyNone NotifyType = iota
	NotifyUnread
	NotifyHighlight
)

type optional int

const (
	optionalUnset optional = iota
	optionalFalse
	optionalTrue
)

type buffer struct {
	netID           string
	netName         string
	title           string
	highlights      int
	notifications   []int
	unread          bool
	unreadLatest    time.Time // latest unread message from a server target snapshot
	snapshotMention bool
	read            time.Time
	openedOnce      bool

	pinned bool
	muted  bool

	// This is the "last read" timestamp when the buffer was last focused.
	// If the "last read" timestamp changes while the buffer is focused,
	// the ruler should not move.
	unreadRuler time.Time
	// Whether to draw the unread bar for the current buffer.
	// The goal is to draw the unread bar iff there was at least one unread
	// message when the buffer was opened.
	// The unreadSkip value starts off as optionalUnset, then gets set to
	// either optionalFalse or optionalTrue when a message is received.
	unreadSkip optional

	lines []Line
	topic StyledString

	scrollAmt   int // offset in lines from the bottom
	topicOffset int // offset in clusters that are skipped when rendering topic text
	isAtTop     bool
}

type BufferList struct {
	ui *UI

	list    []buffer
	overlay *buffer
	current int
	clicked int
	focused bool

	tlInnerWidth int
	tlHeight     int
	textWidth    int

	filterBuffers      bool
	filterBuffersQuery string // lowercased
}

// NewBufferList returns a new BufferList.
// Call Resize() once before using it.
func NewBufferList(ui *UI) BufferList {
	return BufferList{
		ui:      ui,
		list:    []buffer{},
		clicked: -1,
		focused: true,
	}
}

func (bs *BufferList) ResizeTimeline(tlInnerWidth, tlHeight, textWidth int) {
	bs.tlInnerWidth = tlInnerWidth
	bs.tlHeight = tlHeight - 2
	bs.textWidth = textWidth
}

func (bs *BufferList) OpenOverlay() {
	bs.overlay = &buffer{
		netID:   "",
		netName: "",
		title:   Overlay,
	}
}

func (bs *BufferList) CloseOverlay() {
	bs.overlay = nil
}

func (bs *BufferList) HasOverlay() bool {
	return bs.overlay != nil
}

func (bs *BufferList) To(i int) bool {
	bs.overlay = nil
	if len(bs.list) == 0 {
		return false
	}
	if i == bs.current {
		return false
	}
	if 0 <= i {
		bs.current = i
		if len(bs.list) <= bs.current {
			bs.current = len(bs.list) - 1
		}
		bs.clearRead(bs.current)
		b := bs.list[bs.current]
		b.unreadRuler = b.read
		if len(b.lines) > 0 {
			l := b.lines[len(b.lines)-1]
			if !l.At.After(b.unreadRuler) {
				b.unreadSkip = optionalTrue
			} else {
				b.unreadSkip = optionalFalse
			}
		} else {
			b.unreadSkip = optionalUnset
		}
		bs.list[bs.current] = b
		return true
	}
	return false
}

func (bs *BufferList) FilterBuffers(enable bool, query string) {
	bs.filterBuffers = enable
	bs.filterBuffersQuery = strings.ToLower(query)
}

func (bs *BufferList) Next() {
	if len(bs.list) == 0 {
		return
	}
	c := (bs.current + 1) % len(bs.list)
	bs.To(c)
}

func (bs *BufferList) Previous() {
	if len(bs.list) == 0 {
		return
	}
	c := (bs.current - 1 + len(bs.list)) % len(bs.list)
	bs.To(c)
}

func (bs *BufferList) NextUnread() {
	for i := 0; i < len(bs.list); i++ {
		c := (bs.current + i) % len(bs.list)
		if bs.list[c].unread && !bs.list[c].muted {
			bs.To(c)
			return
		}
	}
}

func (bs *BufferList) PreviousUnread() {
	for i := 0; i < len(bs.list); i++ {
		c := (bs.current - i + len(bs.list)) % len(bs.list)
		if bs.list[c].unread && !bs.list[c].muted {
			bs.To(c)
			return
		}
	}
}

func (bs *BufferList) Add(netID, netName, title string) (i int, added bool) {
	if i, b := bs.at(netID, title); b != nil {
		return i, false
	}
	for _, b := range bs.list {
		if netName == "" && b.netID == netID {
			netName = b.netName
			break
		}
	}
	i = 0
	lTitle := strings.ToLower(title)
	for bi, b := range bs.list {
		if b.pinned || b.netName < netName {
			i = bi + 1
			continue
		}
		if b.muted || b.netName > netName {
			break
		}
		lbTitle := strings.ToLower(b.title)
		if lbTitle < lTitle {
			i = bi + 1
			continue
		}
		break
	}

	if i <= bs.current && bs.current < len(bs.list) {
		bs.current++
	}

	b := buffer{
		netID:   netID,
		netName: netName,
		title:   title,
	}
	if i == len(bs.list) {
		bs.list = append(bs.list, b)
	} else {
		bs.list = append(bs.list[:i+1], bs.list[i:]...)
		bs.list[i] = b
	}
	return i, true
}

func (bs *BufferList) Remove(netID, title string) bool {
	idx, b := bs.at(netID, title)
	if b == bs.overlay {
		bs.overlay = nil
		return false
	}
	if idx < 0 {
		return false
	}
	updated := bs.current == idx

	bs.clearRead(idx)
	copy(bs.list[idx:], bs.list[idx+1:])
	bs.list[len(bs.list)-1] = buffer{}
	bs.list = bs.list[:len(bs.list)-1]
	if bs.current >= idx {
		bs.current = max(0, bs.current-1)
	}
	if updated && len(bs.list) > 0 {
		// Force refresh current buffer
		c := bs.current
		bs.current = -1
		bs.To(c)
	}
	return true
}

func (bs *BufferList) RemoveNetwork(netID string) {
	updated := false
	for idx := 0; idx < len(bs.list); idx++ {
		b := &bs.list[idx]
		if b.netID != netID {
			continue
		}
		if idx == bs.current {
			updated = true
		}
		bs.clearRead(idx)
		copy(bs.list[idx:], bs.list[idx+1:])
		bs.list[len(bs.list)-1] = buffer{}
		bs.list = bs.list[:len(bs.list)-1]
		if bs.current >= idx {
			bs.current--
		}
		idx--
	}
	bs.current = max(0, bs.current)
	if updated && len(bs.list) > 0 {
		// Force refresh current buffer
		c := bs.current
		bs.current = -1
		bs.To(c)
	}
}

func (bs *BufferList) reorder() {
	netID, title := bs.Current()
	sort.Slice(bs.list, func(i, j int) bool {
		bi := &bs.list[i]
		bj := &bs.list[j]
		if bi.netID == "" && bj.netID != "" {
			return true
		}
		if bi.netID != "" && bj.netID == "" {
			return false
		}
		if bi.pinned && !bj.pinned {
			return true
		}
		if !bi.pinned && bj.pinned {
			return false
		}
		if c := strings.Compare(bi.netName, bj.netName); c != 0 {
			return c == -1
		}
		if bi.title == "" && bj.title != "" {
			return true
		}
		if bi.title != "" && bj.title == "" {
			return false
		}
		if bi.muted && !bj.muted {
			return false
		}
		if !bi.muted && bj.muted {
			return true
		}
		bti := strings.ToLower(bi.title)
		btj := strings.ToLower(bj.title)
		return strings.Compare(bti, btj) == -1
	})
	i, _ := bs.at(netID, title)
	if i >= 0 {
		bs.current = i
	}
}

func (bs *BufferList) SetTopic(netID, title string, topic StyledString) {
	_, b := bs.at(netID, title)
	if b == nil {
		return
	}
	b.topic = topic
}

func (bs *BufferList) GetPinned(netID, title string) bool {
	_, b := bs.at(netID, title)
	if b == nil {
		return false
	}
	return b.pinned
}

func (bs *BufferList) SetPinned(netID, title string, pinned bool) int {
	_, b := bs.at(netID, title)
	if b == nil {
		return -1
	}
	b.pinned = pinned
	bs.reorder()
	i, _ := bs.at(netID, title)
	return i
}

func (bs *BufferList) GetMuted(netID, title string) bool {
	_, b := bs.at(netID, title)
	if b == nil {
		return false
	}
	return b.muted
}

func (bs *BufferList) SetMuted(netID, title string, muted bool) int {
	_, b := bs.at(netID, title)
	if b == nil {
		return -1
	}
	b.muted = muted
	bs.reorder()
	i, _ := bs.at(netID, title)
	return i
}

func (bs *BufferList) Buffer(i int) (netID, title string, ok bool) {
	if i < 0 || i >= len(bs.list) {
		return
	}
	b := &bs.list[i]
	return b.netID, b.title, true
}

func (bs *BufferList) Current() (netID, title string) {
	b := &bs.list[bs.current]
	return b.netID, b.title
}

func (bs *BufferList) ScrollUp(n int) {
	b := bs.cur()
	if b.isAtTop {
		return
	}
	b.scrollAmt += n
}

func (bs *BufferList) ScrollDown(n int) {
	b := bs.cur()
	b.scrollAmt -= n

	if b.scrollAmt < 0 {
		b.scrollAmt = 0
	}
}

func (bs *BufferList) ScrollUpHighlight() bool {
	b := bs.cur()
	ymin := b.scrollAmt + bs.tlHeight
	return bs.forEachLine(b, func(line *Line, y int) bool {
		if ymin <= y && line.Highlight {
			b.scrollAmt = y - bs.tlHeight + 1
			return true
		}
		return false
	})
}

func (bs *BufferList) ScrollDownHighlight() bool {
	b := bs.cur()
	yLastHighlight := 0
	bs.forEachLine(b, func(line *Line, y int) bool {
		if y >= b.scrollAmt {
			return true
		}
		if line.Highlight {
			yLastHighlight = y
		}
		return false
	})
	b.scrollAmt = yLastHighlight
	return b.scrollAmt != 0
}

func (bs *BufferList) ScrollTopicLeft(n int) {
	b := bs.cur()
	b.topicOffset -= n

	if b.topicOffset < 0 {
		b.topicOffset = 0
	}
}

func (bs *BufferList) ScrollTopicRight(n int) {
	b := bs.cur()
	b.topicOffset += n
}

// LinesAboveOffset returns a rough approximate of the number of lines
// above the offset (that is, starting from the bottom of the screen,
// up to the first line).
func (bs *BufferList) LinesAboveOffset() int {
	b := bs.cur()
	return len(b.lines) - b.scrollAmt
}

func (bs *BufferList) at(netID, title string) (int, *buffer) {
	if netID == "" && title == Overlay {
		return -1, bs.overlay
	}
	lTitle := strings.ToLower(title)
	for i, b := range bs.list {
		if b.netID == netID && strings.ToLower(b.title) == lTitle {
			return i, &bs.list[i]
		}
	}
	return -1, nil
}

func (bs *BufferList) cur() *buffer {
	if bs.overlay != nil {
		return bs.overlay
	}
	return &bs.list[bs.current]
}
