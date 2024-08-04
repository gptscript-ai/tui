package tui

import (
	"strings"
	"sync"
	"time"

	"atomicgo.dev/cursor"
	"github.com/pterm/pterm"
)

var (
	loopDelay = 200 * time.Millisecond
)

type displayState struct {
	area      area
	lastPrint string
	content   func() string
	finish    bool
}

type display struct {
	displayState
	prompter    *prompter
	contentLock sync.Mutex
	paintLock   sync.Mutex
	closer      func()
}

func newDisplay(tool string) (*display, error) {
	prompter, err := newReadlinePrompter(tool)
	if err != nil {
		return nil, err
	}

	t := time.NewTicker(loopDelay)
	d := &display{
		prompter: prompter,
		closer:   t.Stop,
	}

	go func() {
		for range t.C {
			d.paint()
		}
	}()

	return d, nil
}

func (a *display) readline(f func() (string, bool)) (string, bool) {
	a.paint()
	a.paintLock.Lock()
	defer a.paintLock.Unlock()
	cursor.Show()
	defer cursor.Hide()
	return f()
}

func (a *display) Ask(text string, sensitive, allowEmptyResponse bool) (string, bool) {
	a.setMultiLinePrompt(text)
	if sensitive {
		return a.readline(a.prompter.ReadPassword)
	}
	return a.readline(a.prompter.Readline(allowEmptyResponse))
}

func (a *display) setMultiLinePrompt(text string) {
	lines := strings.Split(text, "\n")
	a.prompter.SetPrompt(lines[len(lines)-1])
	if len(lines) > 1 {
		a.contentLock.Lock()
		defer a.contentLock.Unlock()
		newContent := a.getContent() + "\n" + strings.Join(lines[:len(lines)-1], "\n") + "\n"
		a.content = func() string {
			return newContent
		}
	}
}

type Answer string

const (
	Yes    = Answer("Yes")
	No     = Answer("No")
	Always = Answer("Always")
)

func (a *display) AskYesNo(text string) (Answer, bool, error) {
	a.setMultiLinePrompt(text)
	for {
		line, ok := a.readline(a.prompter.Readline(false))
		if !ok {
			return No, ok, nil
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return Yes, true, nil
		case "n", "no":
			return No, true, nil
		case "a", "always":
			return Always, true, nil
		}
	}
}

func (a *display) Prompt(text string) (string, bool) {
	a.prompter.SetPrompt(text)
	return a.readline(a.prompter.Readline(false))
}

func (a *display) getContent() string {
	if a.content == nil {
		return ""
	}
	return a.content()
}

func (a *display) paint() {
	a.paintLock.Lock()
	defer a.paintLock.Unlock()

	a.contentLock.Lock()
	if a.finish {
		a.area.Update(a.getContent())
		a.displayState = displayState{}
		a.contentLock.Unlock()
		return
	}

	newContent := a.getContent()
	a.contentLock.Unlock()

	if newContent == a.lastPrint {
		return
	}

	lines := strings.Split(newContent, "\n")
	height := pterm.GetTerminalHeight()
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	a.area.Update(strings.Join(lines, "\n"))
	a.lastPrint = newContent
}

func (a *display) Progress(text func() string) {
	a.contentLock.Lock()
	defer a.contentLock.Unlock()
	a.content = text
}

func (a *display) Close() error {
	a.closer()
	return a.prompter.Close()
}

func (a *display) Finished(text string) {
	defer a.paint()
	a.contentLock.Lock()
	defer a.contentLock.Unlock()

	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	a.finish = true
	a.content = func() string {
		return text
	}
}
