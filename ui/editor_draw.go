package ui

import (
	"go.rockorager.dev/vaxis"
)

func (e *Editor) Draw(vx *Vaxis, x0, y int, hint string) {
	var st vaxis.Style

	x := x0
	i := e.text[e.lineIdx].clusters[e.offsetIdx]
	text := e.text[e.lineIdx].runes
	showCursor := true

	if len(text) == 0 && len(hint) > 0 && !e.backsearch {
		i = 0
		text = []rune(hint)
		st.Foreground = e.ui.config.Colors.Status
		showCursor = false
	}

	autoStart := -1
	autoEnd := -1
	autoX := x0
	if e.autoCache != nil {
		autoStart = e.autoCache[e.autoCacheIdx].StartIdx
		autoEnd = e.autoCache[e.autoCacheIdx].EndIdx
	}

	ci := e.text[e.lineIdx].clusters[e.cursorIdx]
	for i < len(text) {
		r := text[i:]
		s := st
		if e.backsearch && i < ci && i >= ci-len(e.backsearchPattern) {
			s.UnderlineStyle = vaxis.UnderlineSingle
		}
		if i >= autoStart && i < autoEnd {
			s.UnderlineStyle = vaxis.UnderlineSingle
		}
		if s.UnderlineStyle == vaxis.UnderlineOff {
			for _, t := range e.typos {
				if i >= t.Start && i < t.End {
					s.UnderlineStyle = vaxis.UnderlineCurly
					s.UnderlineColor = e.ui.config.Colors.Gray
					break
				}
			}
		}
		if i == autoStart {
			autoX = x
		}
		if r[0] == '\n' {
			s.Attribute |= vaxis.AttrBold
			s.Foreground = ColorRed
			r = []rune{'↲'}
		}
		dx, di := printCluster(vx, x, y, x0+e.width, r, s)
		if di == 0 {
			break
		}
		x += dx
		i += di
	}
	if i == autoStart {
		autoX = x
	}

	for x < x0+e.width {
		setCell(vx, x, y, ' ', st)
		x++
	}

	autoCount := y - 2
	if autoCount < 0 {
		autoCount = 0
	} else if autoCount > len(e.autoCache) {
		autoCount = len(e.autoCache)
	} else if autoCount > 10 {
		autoCount = 10
	}
	autoOff := 0
	if len(e.autoCache) > autoCount {
		autoOff = e.autoCacheIdx - autoCount/2
		if autoOff < 0 {
			autoOff = 0
		} else if autoOff > len(e.autoCache)-autoCount {
			autoOff = len(e.autoCache) - autoCount
		}
	}

	for ci, completion := range e.autoCache[autoOff : autoOff+autoCount] {
		display := completion.Display
		if display == nil {
			display = completion.Text[completion.StartIdx:]
		}
		var unselectable bool
		if completion.Async != nil {
			display = []rune("Loading...")
			unselectable = true
		} else if (ci == 0 && autoOff > 0) || (ci == autoCount-1 && autoOff+autoCount < len(e.autoCache)) {
			display = []rune("...")
			unselectable = true
		}

		x := autoX
		y := y - ci - 1
		i := 0
		for i < len(display) {
			s := vaxis.Style{
				Attribute: vaxis.AttrReverse,
			}
			if ci+autoOff == e.autoCacheIdx {
				s.Attribute |= vaxis.AttrBold
			} else {
				s.Attribute |= vaxis.AttrDim
			}
			if unselectable {
				s.Attribute |= vaxis.AttrItalic
			}
			dx, di := printCluster(vx, x, y, x0+e.width, display[i:], s)
			if di == 0 {
				break
			}
			x += dx
			i += di
		}
	}

	if showCursor {
		cursorX := x0 + e.textWidth[e.cursorIdx] - e.textWidth[e.offsetIdx]
		vx.ShowCursor(cursorX, y, vaxis.CursorBeam)
	} else {
		vx.HideCursor()
	}
}

// runeOffset returns the rune index of the rune starting at byte pos in string s
func runeOffset(s string, pos int) int {
	n := 0
	for i := range s {
		if i >= pos {
			return n
		}
		n++
	}
	return n
}
