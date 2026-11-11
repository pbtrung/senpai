package ui

func (bs *BufferList) mergeLine(former *Line, addition Line) (keepLine bool) {
	bs.ui.config.MergeLine(former, addition)
	if former.Body.string == "" {
		return false
	}
	former.width = 0
	former.computeSplitPoints(bs.ui.vx)
	return true
}

func (bs *BufferList) AddLine(netID, title string, line Line) {
	_, b := bs.at(netID, title)
	if b == nil {
		return
	}
	current := bs.cur()

	n := len(b.lines)
	line.At = line.At.UTC()

	if !line.Mergeable && b.openedOnce {
		line.Body = line.Body.ParseURLs()
	}

	if line.Mergeable && n != 0 && b.lines[n-1].Mergeable {
		l := &b.lines[n-1]
		if !bs.mergeLine(l, line) {
			b.lines = b.lines[:n-1]
		}
		// TODO change b.scrollAmt if it's not 0 and bs.current is idx.
	} else {
		line.computeSplitPoints(bs.ui.vx)
		b.lines = append(b.lines, line)
		if b == current && 0 < b.scrollAmt {
			b.scrollAmt += len(line.NewLines(bs.ui.vx, bs.textWidth)) + 1
		}
	}

	if line.Notify != NotifyNone && (!bs.focused || b != current) {
		b.unread = true
	}
	if line.Notify == NotifyHighlight && (!bs.focused || b != current) {
		b.highlights++
	}
	if b == current && b.unreadSkip == optionalUnset && len(b.lines) > 0 {
		if b.unreadRuler.IsZero() || !b.lines[len(b.lines)-1].At.After(b.unreadRuler) {
			b.unreadSkip = optionalTrue
		} else {
			b.unreadSkip = optionalFalse
		}
	}
	bs.trimLines(b)
}

func (bs *BufferList) trimLines(b *buffer) {
	if len(b.lines) <= MaxBufferLines {
		return
	}
	// Preserve unread state for messages which can be fetched again later.
	for _, line := range b.lines[:len(b.lines)-MaxBufferLines] {
		if line.Readable && line.At.After(b.read) && line.Notify != NotifyNone {
			if line.At.After(b.unreadLatest) {
				b.unreadLatest = line.At
			}
			b.snapshotMention = b.snapshotMention || line.Notify == NotifyHighlight
		}
	}
	n := copy(b.lines, b.lines[len(b.lines)-MaxBufferLines:])
	clear(b.lines[n:])
	b.lines = b.lines[:n]
	b.isAtTop = false
	if b.scrollAmt > 0 {
		height := 0
		for i := range b.lines {
			height += len(b.lines[i].NewLines(bs.ui.vx, bs.textWidth)) + 1
		}
		b.scrollAmt = min(b.scrollAmt, max(0, height-bs.tlHeight))
	}
}

func (bs *BufferList) AddLines(netID, title string, before, after []Line) {
	_, b := bs.at(netID, title)
	if b == nil {
		return
	}

	lines := make([]Line, 0, len(before)+len(b.lines)+len(after))
	for _, buf := range []*[]Line{&before, &b.lines, &after} {
		for _, line := range *buf {
			if line.Mergeable && len(lines) > 0 && lines[len(lines)-1].Mergeable {
				l := &lines[len(lines)-1]
				if !bs.mergeLine(l, line) {
					lines = lines[:len(lines)-1]
				}
			} else {
				if buf != &b.lines {
					if b.openedOnce {
						line.Body = line.Body.ParseURLs()
					}
					line.computeSplitPoints(bs.ui.vx)
				}
				lines = append(lines, line)
			}
		}
	}
	b.lines = lines
	bs.trimLines(b)
	if (!bs.focused || b != bs.cur()) && !b.read.IsZero() {
		bs.recountUnread(b)
	}
	if b == bs.cur() && b.unreadSkip == optionalUnset && len(b.lines) > 0 {
		if b.unreadRuler.IsZero() || !b.lines[len(b.lines)-1].At.After(b.unreadRuler) {
			b.unreadSkip = optionalTrue
		} else {
			b.unreadSkip = optionalFalse
		}
	}
}
