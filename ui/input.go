package ui

import (
	"git.sr.ht/~delthas/senpai/events"
)

// InputContent result must not be modified.
func (ui *UI) InputContent() []rune {
	return ui.e.Content()
}

func (ui *UI) InputRune(r rune) {
	ui.e.PutRune(r)
}

func (ui *UI) SetTypos(typos []events.TypoRange) {
	ui.e.SetTypos(typos)
}

// InputEnter returns true if the event was eaten
func (ui *UI) InputEnter() bool {
	return ui.e.Enter()
}

func (ui *UI) InputRight() {
	ui.e.Right()
}

func (ui *UI) InputRightWord() {
	ui.e.RightWord()
}

func (ui *UI) InputLeft() {
	ui.e.Left()
}

func (ui *UI) InputLeftWord() {
	ui.e.LeftWord()
}

func (ui *UI) InputHome() {
	ui.e.Home()
}

func (ui *UI) InputEnd() {
	ui.e.End()
}

func (ui *UI) InputUp() {
	ui.e.Up()
}

func (ui *UI) InputDown() {
	ui.e.Down()
}

func (ui *UI) InputBackspace() (ok bool) {
	return ui.e.RemCluster()
}

func (ui *UI) InputDelete() (ok bool) {
	return ui.e.RemClusterForward()
}

func (ui *UI) InputDeleteBefore() (ok bool) {
	return ui.e.RemBefore()
}

func (ui *UI) InputDeleteAfter() (ok bool) {
	return ui.e.RemAfter()
}

func (ui *UI) InputDeleteWord() (ok bool) {
	return ui.e.RemWord()
}

func (ui *UI) InputDeleteNextWord() (ok bool) {
	return ui.e.RemWordForward()
}

func (ui *UI) InputAutoComplete() (ok bool) {
	return ui.e.AutoComplete()
}

func (ui *UI) InputFlush() (content string) {
	return ui.e.Flush()
}

func (ui *UI) InputClear() bool {
	return ui.e.Clear()
}

func (ui *UI) InputSet(text string) {
	ui.e.Set(text)
}

func (ui *UI) InputBackSearch() {
	ui.e.BackSearch()
}

func (ui *UI) AsyncCompletions(id int, cs []Completion) {
	ui.e.AsyncCompletions(id, cs)
}
