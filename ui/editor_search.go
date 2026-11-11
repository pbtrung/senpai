package ui

import (
	"strings"
)

func (e *Editor) BackSearch() {
	if !e.backsearch {
		e.backsearch = true
		e.backsearchPattern = []rune(strings.ToLower(string(e.text[e.lineIdx].runes)))
	}
	e.backsearchUpdate(e.lineIdx - 1)
}

func (e *Editor) backsearchUpdate(start int) {
	if len(e.backsearchPattern) == 0 {
		return
	}
	pattern := string(e.backsearchPattern)
	for i := start; i >= 0; i-- {
		if match := strings.Index(strings.ToLower(string(e.text[i].runes)), pattern); match >= 0 {
			e.lineIdx = i
			e.recompute()
			e.setCursor(runeOffset(string(e.text[i].runes), match) + len(e.backsearchPattern))
			e.offsetIdx = 0
			for e.offsetIdx < len(e.textWidth)-1 && e.width < e.textWidth[e.cursorIdx]-e.textWidth[e.offsetIdx]+16 {
				e.offsetIdx++
			}
			e.autoCache = nil
			break
		}
	}
}

func (e *Editor) backsearchEnd() {
	e.backsearch = false
}

// runeOffset returns the lowercase version of a rune
// TODO: len(strings.ToLower(string(r))) == len(strings.ToUpper(string(r))) for all x?
func runeToLower(r rune) rune {
	return []rune(strings.ToLower(string(r)))[0]
}
