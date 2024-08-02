package tui

import (
	"os"
	"strings"

	"atomicgo.dev/cursor"
)

type area struct {
	content string
	cursor  *cursor.Area
	ignore  bool
}

func (a *area) Write(p []byte) (n int, err error) {
	if a.ignore {
		return len(p), nil
	}
	return os.Stdout.Write(p)
}

func (a *area) Fd() uintptr {
	return os.Stdout.Fd()
}

func (a *area) Update(text string) {
	if a.cursor == nil {
		c := cursor.NewArea().WithWriter(a)
		a.cursor = &c
		cursor.Hide()
	}

	if a.content == text {
		return
	}

	defer func() {
		a.ignore = false
		a.content = text
	}()

	if rest, ok := strings.CutPrefix(text, a.content); ok {
		// Just write suffix
		_, _ = os.Stdout.Write([]byte(rest))
		a.ignore = true
		a.cursor.Update(text)
	} else {
		a.cursor.Update(text)
	}
}
