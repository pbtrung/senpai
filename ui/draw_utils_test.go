package ui

import (
	"go.rockorager.dev/vaxis"
	"testing"
)

func TestNicknameTruncationUsesSeparator(t *testing.T) {
	vx := &Vaxis{Vaxis: &vaxis.Vaxis{}}
	for _, test := range []struct {
		text    string
		width   int
		visible string
		marker  rune
	}{
		{"longnickname", 8, "longnick", '+'},
		{"exactfit", 8, "exactfit", ' '},
		{"short", 8, "short", ' '},
		{"ab界cd", 4, "ab界", '+'},
		{"ab界cd", 3, "ab", '+'},
		{"e\u0301long", 1, "e\u0301", '+'},
		{"nickname", 1, "n", '+'},
		{"", 8, "", ' '},
	} {
		visible, marker := identText(vx, test.text, test.width)
		if visible != test.visible || marker != test.marker {
			t.Errorf("%q width %d: got %q/%q, want %q/%q", test.text, test.width, visible, marker, test.visible, test.marker)
		}
	}
}
