package ui

import (
	"strings"
	"time"
)

func IsSplitRune(r rune) bool {
	return r == ' ' || r == '\t'
}

type point struct {
	X     int // in cells
	I     int // in bytes
	Split bool
}

type NotifyType int

type Line struct {
	At        time.Time
	Head      StyledString
	Body      StyledString
	Notify    NotifyType
	Highlight bool
	Readable  bool
	Mergeable bool
	Data      interface{}

	splitPoints []point
	width       int
	newLines    []int
}

func (l *Line) IsZero() bool {
	return l.Body.string == ""
}

func (l *Line) computeSplitPoints(vx *Vaxis) {
	if l.splitPoints == nil {
		l.splitPoints = []point{}
	}

	width := 0
	lastWasSplit := false
	l.splitPoints = l.splitPoints[:0]

	for i, r := range l.Body.string {
		curIsSplit := IsSplitRune(r)

		if i == 0 || lastWasSplit != curIsSplit {
			l.splitPoints = append(l.splitPoints, point{
				X:     width,
				I:     i,
				Split: curIsSplit,
			})
		}

		lastWasSplit = curIsSplit
		width += runeWidth(vx, r)
	}

	if !lastWasSplit {
		l.splitPoints = append(l.splitPoints, point{
			X:     width,
			I:     len(l.Body.string),
			Split: true,
		})
	}
}

// NewLines returns the offsets, in bytes, where the line should be split.
func (l *Line) NewLines(vx *Vaxis, width int) []int {
	// Beware! This function was made by your local Test Driven Developper™ who
	// doesn't understand one bit of this function and how it works (though it
	// might not work that well if you're here...).  The code below is thus very
	// cryptic and not well-structured.  However, I'm going to try to explain
	// some of those lines!

	if l.width == width {
		return l.newLines
	}
	if l.newLines == nil {
		l.newLines = []int{}
	}
	l.newLines = l.newLines[:0]
	l.width = width

	// Wrap each explicit line independently, keeping byte offsets into the
	// original styled body so links and formatting survive multiline messages.
	if strings.ContainsRune(l.Body.string, '\n') {
		offset := 0
		parts := strings.Split(l.Body.string, "\n")
		for i, part := range parts {
			segment := Line{Body: PlainString(part)}
			segment.computeSplitPoints(vx)
			for _, split := range segment.NewLines(vx, width) {
				l.newLines = append(l.newLines, offset+split)
			}
			offset += len(part)
			if i < len(parts)-1 {
				l.newLines = append(l.newLines, offset)
				offset++
			}
		}
		return l.newLines
	}

	x := 0
	for i := 1; i < len(l.splitPoints); i++ {
		// Iterate through the split points 2 by 2.  Split points are placed at
		// the beginning of whitespace (see IsSplitRune) and at the beginning
		// of non-whitespace. Iterating on 2 points each time, sp1 and sp2,
		// allows consideration of a "word" of (non-)whitespace.
		// Split points have the index I in the string and the width X of the
		// screen.  Finally, the Split field is set to true if the split point
		// is at the beginning of a whitespace.

		// Below, "row" means a line in the terminal, while "line" means (l *Line).

		sp1 := l.splitPoints[i-1]
		sp2 := l.splitPoints[i]

		if 0 < len(l.newLines) && x == 0 && sp1.Split {
			// Except for the first row, let's skip the whitespace at the start
			// of the row.
		} else if !sp1.Split && sp2.X-sp1.X == width {
			// Some word occupies the width of the terminal, lets place a
			// newline at the PREVIOUS split point (i-2, which is whitespace)
			// ONLY if there isn't already one.
			if 1 < i && (0 == len(l.newLines) || (l.newLines[len(l.newLines)-1] != l.splitPoints[i-2].I && l.newLines[len(l.newLines)-1] != l.splitPoints[i-1].I)) {
				l.newLines = append(l.newLines, l.splitPoints[i-2].I)
			}
			// and also place a newline after the word.
			x = 0
			l.newLines = append(l.newLines, sp2.I)
		} else if sp2.X-sp1.X+x < width {
			// It fits.  Advance the X coordinate with the width of the word.
			x += sp2.X - sp1.X
		} else if sp2.X-sp1.X+x == width {
			// It fits, but there is no more space in the row.
			x = 0
			l.newLines = append(l.newLines, sp2.I)
		} else if sp1.Split && width < sp2.X-sp1.X {
			// Some whitespace occupies a width larger than the terminal's.
			x = 0
			l.newLines = append(l.newLines, sp1.I)
		} else if width < sp2.X-sp1.X {
			// It doesn't fit at all.  The word is longer than the width of the
			// terminal.  In this case, no newline is placed before (like in the
			// 2nd if-else branch).  The for loop is used to place newlines in
			// the word.
			s := l.Body.string[sp1.I:sp2.I]
			j := 0
			for s != "" {
				c, wordWidth := firstCluster(vx, []rune(s))
				if width < x+wordWidth {
					x = 0
					l.newLines = append(l.newLines, sp1.I+j)
				}
				x += wordWidth
				j += len(c)
				s = s[len(c):]
			}
			if x == width {
				// The placement of the word is such that it ends right at the
				// end of the row.
				x = 0
				l.newLines = append(l.newLines, sp2.I)
			}
		} else {
			// So... IIUC this branch would be the same as
			//     else if width < sp2.X-sp1.X+x
			// IE. It doesn't fit, but the word can still be placed on the next
			// row.
			l.newLines = append(l.newLines, sp1.I)
			if sp1.Split {
				x = 0
			} else {
				x = sp2.X - sp1.X
			}
		}
	}

	if 0 < len(l.newLines) && l.newLines[len(l.newLines)-1] == len(l.Body.string) {
		// DROP any newline that is placed at the end of the string because we
		// don't care about those.
		l.newLines = l.newLines[:len(l.newLines)-1]
	}

	return l.newLines
}
