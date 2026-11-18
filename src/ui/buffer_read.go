package ui

import (
	"time"
)

func (bs *BufferList) Focused() bool {
	return bs.focused
}

func (bs *BufferList) SetFocused(focused bool) {
	bs.focused = focused
	if focused {
		bs.clearRead(bs.current)
	}
}

func (bs *BufferList) clearRead(i int) {
	b := &bs.list[i]
	b.highlights = 0
	b.unread = false
	// Keep the snapshot until a read marker covers it. Merely selecting a
	// buffer does not mean its unloaded history has been read.
	if len(b.notifications) > 0 {
		for _, id := range b.notifications {
			notifyClose(id)
		}
		b.notifications = b.notifications[:0]
	}
}

// SetUnread applies a server snapshot without downloading the target's history.
func (bs *BufferList) SetUnread(netID, title string, latest time.Time, unread, mention bool) {
	_, b := bs.at(netID, title)
	if b == nil || !latest.After(b.read) {
		return
	}
	if unread {
		b.unreadLatest = latest
		b.snapshotMention = mention
		if !bs.focused || b != bs.cur() {
			b.unread = true
			if mention && b.highlights == 0 {
				b.highlights = 1
			}
		}
	}
}

// recountUnread replaces a snapshot's placeholder count with the loaded unread
// messages. Re-reading history must not count the same highlight again.
func (bs *BufferList) recountUnread(b *buffer) {
	b.unread = b.unreadLatest.After(b.read)
	b.highlights = 0
	for i := len(b.lines) - 1; i >= 0; i-- {
		line := &b.lines[i]
		if !line.At.After(b.read) {
			break
		}
		if line.Readable {
			if line.Notify != NotifyNone {
				b.unread = true
			}
			if line.Notify == NotifyHighlight {
				b.highlights++
			}
		}
	}
	if b.snapshotMention && b.unreadLatest.After(b.read) && b.highlights == 0 {
		b.highlights = 1
	}
}

func (bs *BufferList) SetRead(netID, title string, timestamp time.Time) {
	i, b := bs.at(netID, title)
	if b == nil || i < 0 || timestamp.Before(b.read) {
		return
	}
	b.read = timestamp
	if !b.unreadLatest.After(timestamp) {
		b.unreadLatest = time.Time{}
		b.snapshotMention = false
	}
	if bs.focused && b == bs.cur() {
		bs.clearRead(i)
	} else {
		bs.recountUnread(b)
		if !b.unread {
			bs.clearRead(i)
		}
	}
	if b.unreadRuler.IsZero() {
		b.unreadRuler = b.read
	}
}

func (bs *BufferList) UpdateRead() (netID, title string, timestamp time.Time) {
	// Search results are temporary and have no server-side read marker.
	if bs.HasOverlay() {
		return "", "", time.Time{}
	}
	b := bs.cur()
	var l *Line
	bs.forEachLine(b, func(line *Line, y int) bool {
		if y >= b.scrollAmt && line.Readable {
			l = line
			return true
		}
		return false
	})
	if l != nil && l.At.After(b.read) {
		b.read = l.At
		return b.netID, b.title, b.read
	}
	return "", "", time.Time{}
}

// ReadMarker identifies a conversation's read progress for server synchronization.
type ReadMarker struct {
	NetID     string
	Buffer    string
	Timestamp time.Time
}

// MarkAllRead clears every real buffer, including muted and unloaded ones.
// It uses known message times rather than the wall clock, so clock skew cannot
// mark messages which have not arrived yet as read. Search overlays are excluded.
func (bs *BufferList) MarkAllRead() []ReadMarker {
	var markers []ReadMarker
	for i := range bs.list {
		b := &bs.list[i]
		latest := b.read
		if b.unreadLatest.After(latest) {
			latest = b.unreadLatest
		}
		for _, line := range b.lines {
			if line.Readable && line.At.After(latest) {
				latest = line.At
			}
		}
		if b.title != "" && latest.After(b.read) {
			markers = append(markers, ReadMarker{b.netID, b.title, latest})
		}
		b.read = latest
		b.unreadLatest = time.Time{}
		b.snapshotMention = false
		b.unreadRuler = latest
		b.unreadSkip = optionalTrue
		bs.clearRead(i)
	}
	return markers
}

func (bs *BufferList) Highlights() int {
	n := 0
	for _, b := range bs.list {
		n += b.highlights
	}
	return n
}
