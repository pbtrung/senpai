package ui

import (
	"fmt"
	"math"
	"strings"

	"go.rockorager.dev/vaxis"
)

func (bs *BufferList) DrawVerticalBufferList(vx *Vaxis, x0, y0, width, height int, offset *int) {
	if y0+len(bs.list)-*offset < height {
		*offset = y0 + len(bs.list) - height
		if *offset < 0 {
			*offset = 0
		}
	}
	off := bs.VerticalBufferOffset(0, *offset)
	if off < 0 {
		off = len(bs.list)
	}

	width--
	bs.ui.drawVerticalLine(vx, x0+width, y0, height)
	clearArea(vx, x0, y0, width, height)

	indexPadding := 1 + int(math.Ceil(math.Log10(float64(len(bs.list)))))
	y := y0
	for i, b := range bs.list[off:] {
		bi := off + i
		x := x0
		var st vaxis.Style
		if b.unread && !b.muted {
			st.Attribute |= vaxis.AttrBold
			st.Foreground = bs.ui.config.Colors.Unread
		}
		if bi == bs.current || bi == bs.clicked {
			st.Attribute |= vaxis.AttrReverse
		} else if b.muted {
			st.Foreground = bs.ui.config.Colors.Gray
		}

		var title string
		if b.title == "" {
			title = b.netName
		} else {
			title = b.title
		}

		if bs.filterBuffers {
			if !strings.Contains(strings.ToLower(title), bs.filterBuffersQuery) {
				continue
			}
			indexSt := st
			indexSt.Foreground = bs.ui.config.Colors.Gray
			indexText := fmt.Sprintf("%d:", bi+1)
			printString(vx, &x, y, Styled(indexText, indexSt))
			x = x0 + indexPadding
		}

		if b.title != "" {
			var st vaxis.Style
			if bi == bs.current || bi == bs.clicked {
				st.Attribute |= vaxis.AttrReverse
			}
			if b.pinned {
				st.Attribute |= vaxis.AttrBold
				setCell(vx, x, y, '⚲', st)
				setCell(vx, x+1, y, ' ', st)
			} else {
				setCell(vx, x, y, ' ', st)
				setCell(vx, x+1, y, ' ', st)
			}
			x += 2
		}
		title = truncate(vx, title, width-(x-x0), "\u2026")
		printString(vx, &x, y, Styled(title, st))

		if bi == bs.current || bi == bs.clicked {
			st := vaxis.Style{
				Attribute: vaxis.AttrReverse,
			}
			for ; x < x0+width; x++ {
				setCell(vx, x, y, ' ', st)
			}
			setCell(vx, x, y, ' ', st)
			setCell(vx, x, y, '▐', st)
		}

		if b.highlights != 0 {
			highlightSt := st
			highlightSt.Foreground = ColorRed
			highlightSt.Attribute |= vaxis.AttrReverse
			highlightText := fmt.Sprintf(" %d ", b.highlights)
			x = x0 + width - len(highlightText)
			printString(vx, &x, y, Styled(highlightText, highlightSt))
		}

		y++
	}
}

func (bs *BufferList) HorizontalBufferOffset(x int, offset int) int {
	if bs.filterBuffers {
		offset = 0
	}
	i := 0
	for bi, b := range bs.list[offset:] {
		if bs.filterBuffers {
			var title string
			if b.title == "" {
				title = b.netName
			} else {
				title = b.title
			}
			if !strings.Contains(strings.ToLower(title), bs.filterBuffersQuery) {
				continue
			}
		}
		if i > 0 {
			x--
			if x < 0 {
				return -1
			}
		}
		x -= bs.bufferWidth(&b)
		if x < 0 {
			return offset + bi
		}
		i++
	}
	return -1
}

func (bs *BufferList) VerticalBufferOffset(y int, offset int) int {
	if !bs.filterBuffers {
		return offset + y
	}

	for i, b := range bs.list {
		var title string
		if b.title == "" {
			title = b.netName
		} else {
			title = b.title
		}

		if bs.filterBuffers {
			if !strings.Contains(strings.ToLower(title), bs.filterBuffersQuery) {
				continue
			}
		}
		if y == 0 {
			return i
		}
		y--
	}
	return -1
}

func (bs *BufferList) GetLeftMost(screenWidth int) int {
	if len(bs.list) == 0 {
		return 0
	}

	width := 0
	var leftMost int

	for leftMost = bs.current; leftMost >= 0; leftMost-- {
		if leftMost < bs.current {
			width++
		}
		width += bs.bufferWidth(&bs.list[leftMost])
		if width > screenWidth {
			return leftMost + 1 // Went offscreen, need to go one step back
		}
	}

	return 0
}

func (bs *BufferList) bufferWidth(b *buffer) int {
	width := 0
	if b.title == "" {
		width += stringWidth(bs.ui.vx, b.netName)
	} else {
		width += stringWidth(bs.ui.vx, b.title)
	}
	if 0 < b.highlights {
		width += 2 + len(fmt.Sprintf("%d", b.highlights))
	}
	return width
}

func (bs *BufferList) DrawHorizontalBufferList(vx *Vaxis, x0, y0, width int, offset *int) {
	x := width
	for i := len(bs.list) - 1; i >= 0; i-- {
		b := &bs.list[i]
		x--
		x -= bs.bufferWidth(b)
		if x <= 10 {
			break
		}
		if *offset > i {
			*offset = i
		}
	}
	x = x0

	off := bs.HorizontalBufferOffset(0, *offset)
	if off < 0 {
		off = len(bs.list)
	}

	for i, b := range bs.list[off:] {
		i := i + off
		if width <= x-x0 {
			break
		}
		var st vaxis.Style
		if b.unread && !b.muted {
			st.Attribute |= vaxis.AttrBold
			st.Foreground = bs.ui.config.Colors.Unread
		} else if i == bs.current {
			st.UnderlineStyle = vaxis.UnderlineSingle
		}
		if i == bs.clicked {
			st.Attribute |= vaxis.AttrReverse
		} else if b.muted {
			st.Foreground = bs.ui.config.Colors.Gray
		}

		var title string
		if b.title == "" {
			st.Attribute |= vaxis.AttrDim
			title = b.netName
		} else {
			title = b.title
		}

		if bs.filterBuffers {
			if !strings.Contains(strings.ToLower(title), bs.filterBuffersQuery) {
				continue
			}
		}

		title = truncate(vx, title, width-x, "\u2026")
		printString(vx, &x, y0, Styled(title, st))

		if 0 < b.highlights {
			st.Foreground = ColorRed
			st.Attribute |= vaxis.AttrReverse
			setCell(vx, x, y0, ' ', st)
			x++
			printNumber(vx, &x, y0, st, b.highlights)
			setCell(vx, x, y0, ' ', st)
			x++
		}
		setCell(vx, x, y0, ' ', vaxis.Style{})
		x++
	}
	for x < width {
		setCell(vx, x, y0, ' ', vaxis.Style{})
		x++
	}
}
