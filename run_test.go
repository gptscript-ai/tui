package tui

import (
	"testing"
)

func TestSplitAtTerm(t *testing.T) {
	testCases := []struct {
		name  string
		line  string
		width int
		want  string
	}{
		{
			name:  "NormalCase",
			line:  "abcdef ghi jklmnopq rst",
			width: 10,
			want:  "abcdef ghi\njklmnopq\nrst",
		},
		{
			name:  "NormalCasePlusOne",
			line:  "ab def ghii jklmnopq r",
			width: 10,
			want:  "ab def\nghii\njklmnopq r",
		},
		{
			name:  "SingleWordLongerThanWidth",
			line:  "abcdefghijklmno",
			width: 10,
			want:  "abcdefghijklmno",
		},
		{
			name:  "TwoWordLongerThanWidth",
			line:  "abcdefghijklmno abcdefghijklmno",
			width: 10,
			want:  "abcdefghijklmno\nabcdefghijklmno",
		},
		{
			name:  "EmptyLine",
			line:  "",
			width: 10,
			want:  "",
		},
		{
			name:  "ZeroWidth",
			line:  "abcdef ghi jklmnopq rst",
			width: 0,
			want:  "abcdef\nghi\njklmnopq\nrst",
		},
		{
			name:  "Whitespace",
			line:  "					",
			width: 4,
			want:  "",
		},
		{
			name:  "SingleCharacter",
			line:  "a",
			width: 10,
			want:  "a",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitAtTerm(tc.line, tc.width)
			if got != tc.want {
				t.Errorf("\nsplitAtTerm( %v , %v ) \ngot: %v, \nwant: %v", tc.line, tc.width, got, tc.want)
			}
		})
	}
}
