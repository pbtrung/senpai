package ui

import (
	"git.sr.ht/~delthas/senpai/events"
	"go.rockorager.dev/vaxis"
)

type editorLine struct {
	runes    []rune
	clusters []int
}

func newEditorLine() editorLine {
	return editorLine{
		runes:    []rune{},
		clusters: []int{0},
	}
}

func (l *editorLine) copy() editorLine {
	return editorLine{
		runes:    append([]rune{}, l.runes...),
		clusters: append([]int{}, l.clusters...),
	}
}

// Editor is the text field where the user writes messages and commands.
type Editor struct {
	ui *UI

	// text contains the written runes. An empty slice means no text is written.
	text []editorLine

	// history contains the original content of each previously sent message.
	// It gets out of sync with text when history is modified à la readline,
	// then overwrites text when a new message is added.
	history []editorLine

	lineIdx int

	// textWidth[i] contains the width of text.runes[:text.clusters[i]]. Therefore
	// len(textWidth) is always strictly greater than 0 and textWidth[0] is
	// always 0.
	textWidth []int

	// cursorIdx is the index of clusters in text of the placement of the cursor.
	cursorIdx int

	// offsetIdx is the number of clusters in text that are skipped when rendering.
	offsetIdx int

	// width is the width of the screen.
	width int

	autoCache    []Completion
	autoCacheIdx int

	backsearch        bool
	backsearchPattern []rune // pre-lowercased

	// oldest (lowest) index in text of lines that were changed.
	// used as an optimization to reduce copying when flushing lines.
	oldestTextChange int

	typos []events.TypoRange
}

// NewEditor returns a new Editor.
// Call Resize() once before using it.
func NewEditor(ui *UI) Editor {
	return Editor{
		ui:        ui,
		text:      []editorLine{newEditorLine()},
		history:   []editorLine{},
		textWidth: []int{0},
	}
}

func (e *Editor) Resize(width int) {
	if width < e.width {
		// Reset cursor to the same size, to recompute offsetIdx
		e.setCursor(e.textWidth[e.cursorIdx])
	}
	e.width = width
}

// Content result must not be modified.
func (e *Editor) Content() []rune {
	return e.text[e.lineIdx].runes
}

func (e *Editor) Empty() bool {
	return len(e.text[e.lineIdx].runes) == 0
}

// recompute must be called when runes is changed, to update
// clusters, textWidth.
func (e *Editor) recompute() {
	c := make([]int, 0, len(e.text[e.lineIdx].runes)+1)
	w := make([]int, 0, len(e.text[e.lineIdx].runes)+1)
	nc := 0
	nw := 0
	it := vaxis.NewCharacterIterator(string(e.text[e.lineIdx].runes))
	for {
		g, ok := it.Next()
		if !ok {
			break
		}
		c = append(c, nc)
		w = append(w, nw)
		nc += len([]rune(g.Grapheme))
		nw += stringWidth(e.ui.vx, g.Grapheme)
	}
	c = append(c, nc)
	w = append(w, nw)
	e.text[e.lineIdx].clusters = c
	e.textWidth = w
}

// setCursor sets cursorIdx to the (grapheme cluster) offset
// corresponding to the passed rune offset runeIdx, rounding up
// to the next grapheme cluster as needed.
func (e *Editor) setCursor(runeIdx int) {
	for i, o := range e.text[e.lineIdx].clusters {
		if o >= runeIdx {
			e.cursorIdx = i
			break
		}
	}
	if e.width <= e.textWidth[e.cursorIdx]-e.textWidth[e.offsetIdx] {
		e.offsetIdx += 16
		// If we went too far right, go back enough to show one cluster.
		max := len(e.text[e.lineIdx].clusters) - 2
		if max < e.offsetIdx {
			e.offsetIdx = max
		}
	}
}

func (e *Editor) PutRune(r rune) {
	e.autoCache = nil
	lowerRune := runeToLower(r)
	if e.backsearch && e.cursorIdx < len(e.text[e.lineIdx].clusters)-1 {
		lowerNext := runeToLower(e.text[e.lineIdx].runes[e.text[e.lineIdx].clusters[e.cursorIdx]])
		if lowerRune == lowerNext {
			e.right()
			e.backsearchPattern = append(e.backsearchPattern, lowerRune)
			return
		}
	}
	e.putRune(r)
	if e.backsearch {
		wasEmpty := len(e.backsearchPattern) == 0
		e.backsearchPattern = append(e.backsearchPattern, lowerRune)
		if wasEmpty {
			e.backsearchUpdate(e.lineIdx - 1)
		} else {
			e.backsearchUpdate(e.lineIdx)
		}
	}
}

// putRune inserts a rune at the current cursor position,
// then updates moves the cursor position after that rune.
// (If inserting the rune merged a grapheme cluster, we
// move the cursor after that cluster.)
func (e *Editor) putRune(r rune) {
	ci := e.text[e.lineIdx].clusters[e.cursorIdx]
	e.text[e.lineIdx].runes = append(e.text[e.lineIdx].runes, ' ')
	copy(e.text[e.lineIdx].runes[ci+1:], e.text[e.lineIdx].runes[ci:])
	e.text[e.lineIdx].runes[ci] = r

	e.recompute()
	e.setCursor(ci + 1)

	e.bumpOldestChange()
}

func (e *Editor) RemCluster() (ok bool) {
	ok = 0 < e.cursorIdx
	if !ok {
		return
	}
	e.remClusterAt(e.cursorIdx - 1)
	e.left()
	e.autoCache = nil
	if e.backsearch {
		if e.Empty() || len(e.backsearchPattern) == 0 {
			e.backsearchEnd()
		} else {
			e.backsearchPattern = e.backsearchPattern[:len(e.backsearchPattern)-1]
			e.backsearchUpdate(e.lineIdx)
		}
	}
	return
}

func (e *Editor) RemClusterForward() (ok bool) {
	ok = e.cursorIdx < len(e.text[e.lineIdx].clusters)-1
	if !ok {
		return
	}
	e.remClusterAt(e.cursorIdx)
	e.autoCache = nil
	e.backsearchEnd()
	return
}

func (e *Editor) RemBefore() (ok bool) {
	ok = e.cursorIdx > 0
	if !ok {
		return
	}
	e.text[e.lineIdx].runes = e.text[e.lineIdx].runes[e.text[e.lineIdx].clusters[e.cursorIdx]:]
	e.cursorIdx = 0
	e.offsetIdx = 0

	e.recompute()
	e.bumpOldestChange()
	e.autoCache = nil
	e.backsearchEnd()
	return
}

func (e *Editor) RemAfter() (ok bool) {
	ok = e.cursorIdx < len(e.text[e.lineIdx].clusters)-1
	if !ok {
		return
	}
	e.text[e.lineIdx].runes = e.text[e.lineIdx].runes[:e.text[e.lineIdx].clusters[e.cursorIdx]]

	e.recompute()
	e.bumpOldestChange()
	e.autoCache = nil
	e.backsearchEnd()
	return
}

func (e *Editor) remClusterAt(idx int) {
	rs := e.text[e.lineIdx].clusters[idx]
	re := e.text[e.lineIdx].clusters[idx+1]
	copy(e.text[e.lineIdx].runes[rs:], e.text[e.lineIdx].runes[re:])
	e.text[e.lineIdx].runes = e.text[e.lineIdx].runes[:len(e.text[e.lineIdx].runes)-(re-rs)]

	e.recompute()
	e.bumpOldestChange()
}

func (e *Editor) RemWord() (ok bool) {
	ok = 0 < e.cursorIdx
	if !ok {
		return
	}

	line := e.text[e.lineIdx]

	// To allow doing something like this (| is the cursor):
	// Hello world|
	// Hello |
	// |
	for e.cursorIdx > 0 && line.runes[line.clusters[e.cursorIdx-1]] == ' ' {
		e.remClusterAt(e.cursorIdx - 1)
		e.left()
	}

	for i := e.cursorIdx - 1; i >= 0; i -= 1 {
		if line.runes[line.clusters[i]] == ' ' {
			break
		}
		e.remClusterAt(i)
		e.left()
	}

	e.autoCache = nil
	e.backsearchEnd()
	return
}

func (e *Editor) RemWordForward() (ok bool) {
	ok = e.cursorIdx < len(e.text[e.lineIdx].clusters)-1
	if !ok {
		return
	}

	for e.cursorIdx < len(e.text[e.lineIdx].clusters)-1 && e.text[e.lineIdx].runes[e.text[e.lineIdx].clusters[e.cursorIdx]] == ' ' {
		e.remClusterAt(e.cursorIdx)
	}

	for e.cursorIdx < len(e.text[e.lineIdx].clusters)-1 {
		if e.text[e.lineIdx].runes[e.text[e.lineIdx].clusters[e.cursorIdx]] == ' ' {
			break
		}
		e.remClusterAt(e.cursorIdx)
	}

	e.autoCache = nil
	e.backsearchEnd()
	return
}

func (e *Editor) Flush() string {
	l := e.text[e.lineIdx]
	content := string(l.runes)
	if len(content) > 0 {
		e.history = append(e.history, l.copy())
	}
	for i, line := range e.history[e.oldestTextChange:] {
		i := i + e.oldestTextChange
		e.text[i] = line.copy()
	}
	if len(content) > 0 {
		e.text = append(e.text, newEditorLine())
	} else {
		e.text[len(e.text)-1] = newEditorLine()
	}
	e.lineIdx = len(e.text) - 1
	e.textWidth = e.textWidth[:1]
	e.cursorIdx = 0
	e.offsetIdx = 0
	e.autoCache = nil
	e.typos = nil
	e.backsearchEnd()
	e.oldestTextChange = len(e.text) - 1
	return content
}

func (e *Editor) Clear() bool {
	if e.Empty() {
		return false
	}
	e.text[e.lineIdx] = newEditorLine()
	e.bumpOldestChange()
	e.textWidth = e.textWidth[:1]
	e.cursorIdx = 0
	e.offsetIdx = 0
	e.autoCache = nil
	return true
}

func (e *Editor) Set(text string) {
	r := []rune(text)
	e.text[e.lineIdx].runes = r
	e.recompute()
	e.bumpOldestChange()
	e.cursorIdx = len(e.text[e.lineIdx].clusters) - 1
	e.offsetIdx = 0
	for e.offsetIdx < len(e.textWidth)-1 && e.width < e.textWidth[e.cursorIdx]-e.textWidth[e.offsetIdx]+16 {
		e.offsetIdx++
	}
	e.autoCache = nil
	e.backsearchEnd()
}

func (e *Editor) Enter() bool {
	if e.autoCache != nil {
		return e.AutoComplete()
	}
	return false
}

func (e *Editor) Right() {
	e.right()
	e.autoCache = nil
	e.backsearchEnd()
}

func (e *Editor) right() {
	if e.cursorIdx == len(e.text[e.lineIdx].clusters)-1 {
		return
	}
	e.cursorIdx++
	if e.width <= e.textWidth[e.cursorIdx]-e.textWidth[e.offsetIdx] {
		e.offsetIdx += 16
		// If we went too far right, go back enough to show one cluster.
		max := len(e.text[e.lineIdx].clusters) - 2
		if max < e.offsetIdx {
			e.offsetIdx = max
		}
	}
}

func (e *Editor) RightWord() {
	line := e.text[e.lineIdx]

	if e.cursorIdx == len(line.clusters)-1 {
		return
	}

	for e.cursorIdx < len(line.clusters)-1 && line.runes[line.clusters[e.cursorIdx]] == ' ' {
		e.Right()
	}
	for i := e.cursorIdx; i < len(line.clusters)-1 && line.runes[line.clusters[i]] != ' '; i++ {
		e.Right()
	}
}

func (e *Editor) Left() {
	e.left()
	e.backsearchEnd()
}

func (e *Editor) left() {
	if e.cursorIdx == 0 {
		return
	}
	e.cursorIdx--
	if e.cursorIdx <= e.offsetIdx {
		e.offsetIdx -= 16
		if e.offsetIdx < 0 {
			e.offsetIdx = 0
		}
	}
}

func (e *Editor) LeftWord() {
	if e.cursorIdx == 0 {
		return
	}

	line := e.text[e.lineIdx]

	for e.cursorIdx > 0 && line.runes[line.clusters[e.cursorIdx-1]] == ' ' {
		e.left()
	}
	for i := e.cursorIdx - 1; i >= 0 && line.runes[line.clusters[i]] != ' '; i-- {
		e.left()
	}

	e.autoCache = nil
	e.backsearchEnd()
}

func (e *Editor) Home() {
	if e.cursorIdx == 0 {
		return
	}
	e.cursorIdx = 0
	e.offsetIdx = 0
	e.autoCache = nil
	e.backsearchEnd()
}

func (e *Editor) End() {
	if e.cursorIdx == len(e.text[e.lineIdx].clusters)-1 {
		return
	}
	e.cursorIdx = len(e.text[e.lineIdx].clusters) - 1
	for e.offsetIdx < len(e.textWidth)-1 && e.width < e.textWidth[e.cursorIdx]-e.textWidth[e.offsetIdx]+16 {
		e.offsetIdx++
	}
	e.autoCache = nil
	e.backsearchEnd()
}

func (e *Editor) Up() {
	if e.autoCache != nil {
		e.autoCacheIdx = (e.autoCacheIdx + 1) % len(e.autoCache)
		return
	}
	if e.lineIdx == 0 {
		return
	}
	e.lineIdx--
	e.recompute()
	e.cursorIdx = 0
	e.offsetIdx = 0
	e.autoCache = nil
	e.backsearchEnd()
	e.End()
}

func (e *Editor) Down() {
	if e.autoCache != nil {
		e.autoCacheIdx = (e.autoCacheIdx + len(e.autoCache) - 1) % len(e.autoCache)
		return
	}
	if e.lineIdx == len(e.text)-1 {
		e.Flush()
		return
	}
	e.lineIdx++
	e.recompute()
	e.cursorIdx = 0
	e.offsetIdx = 0
	e.autoCache = nil
	e.backsearchEnd()
	e.End()
}

// call this everytime e.text is modified
func (e *Editor) bumpOldestChange() {
	if e.oldestTextChange > e.lineIdx {
		e.oldestTextChange = e.lineIdx
	}
}

func (e *Editor) SetTypos(typos []events.TypoRange) {
	e.typos = typos
}
