package ui

type Completion struct {
	Async   any // not nil if this completion is asynchronously loading values
	AsyncID int

	StartIdx  int
	EndIdx    int
	Text      []rune
	Display   []rune
	CursorIdx int // in runes
}

func (e *Editor) AutoComplete() (ok bool) {
	if e.autoCache == nil {
		e.autoCache = e.ui.config.AutoComplete(e.text[e.lineIdx].clusters[e.cursorIdx], e.text[e.lineIdx].runes)
		if len(e.autoCache) == 0 {
			e.autoCache = nil
			return false
		}
		e.autoCacheIdx = 0
		if len(e.autoCache) > 1 || e.autoCache[0].Async != nil {
			return false
		}
	}
	if e.autoCache[e.autoCacheIdx].Async != nil {
		return false
	}

	e.text[e.lineIdx].runes = e.autoCache[e.autoCacheIdx].Text
	e.recompute()
	e.bumpOldestChange()
	e.setCursor(e.autoCache[e.autoCacheIdx].CursorIdx)
	if len(e.textWidth) <= e.offsetIdx {
		e.offsetIdx = 0
	}
	for e.offsetIdx < len(e.textWidth)-1 && e.width < e.textWidth[e.cursorIdx]-e.textWidth[e.offsetIdx]+16 {
		e.offsetIdx++
	}
	e.autoCache = nil

	e.backsearchEnd()
	return true
}

func (e *Editor) AsyncCompletions(id int, cs []Completion) {
	for i := 0; i < len(e.autoCache); i++ {
		c := &e.autoCache[i]
		if c.AsyncID != id {
			continue
		}
		a := append([]Completion{}, e.autoCache[:i]...)
		a = append(a, cs...)
		a = append(a, e.autoCache[i+1:]...)
		e.autoCache = a
		break
	}
	if len(e.autoCache) == 0 {
		e.autoCache = nil
		e.autoCacheIdx = 0
	} else if e.autoCacheIdx >= len(e.autoCache) {
		e.autoCacheIdx = len(e.autoCache) - 1
	}
}
