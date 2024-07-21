package tui

import (
	"regexp"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

var (
	stripControl    = regexp.MustCompile("( ?\x1b\\[[0-9;]+m ?)+\n+$")
	defaultDuration = 200 * time.Millisecond
)

type display struct {
	area         area
	prompter     *prompter
	last         time.Time
	lastDuration time.Duration
	stopped      bool
}

func newDisplay(tool string) (*display, error) {
	prompter, err := newReadlinePrompter(tool)
	if err != nil {
		return nil, err
	}

	return &display{
		prompter:     prompter,
		lastDuration: defaultDuration,
	}, nil
}

func (a *display) Ask(text string, sensitive bool) (string, bool, error) {
	a.setMultiLinePrompt(text)
	if sensitive {
		return a.prompter.ReadPassword()
	}
	return a.prompter.Readline()
}

func (a *display) setMultiLinePrompt(text string) {
	lines := strings.Split(text, "\n")
	a.prompter.SetPrompt(lines[len(lines)-1])
	if len(lines) > 1 {
		a.area.Update(a.area.content + "\n" + strings.Join(lines[:len(lines)-1], "\n") + "\n")
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
		line, ok, err := a.prompter.Readline()
		if !ok || err != nil {
			return No, ok, err
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

func (a *display) Prompt(text string) (string, bool, error) {
	a.prompter.SetPrompt(text)
	return a.prompter.Readline()
}

func (a *display) Progress(text string) error {
	if text == "" {
		return nil
	}

	text = stripControl.ReplaceAllString(text, "")

	if a.stopped {
		a.area = area{}
		a.stopped = false
		a.last = time.Time{}
		a.lastDuration = defaultDuration
	}

	start := time.Now()
	if start.Sub(a.last) > a.lastDuration {
		lines := strings.Split(text, "\n")
		height := pterm.GetTerminalHeight()
		if len(lines) > height {
			lines = lines[len(lines)-height:]
		}
		newText := strings.Join(lines, "\n")
		a.area.Update(newText)
		done := time.Now()
		delta := done.Sub(start)
		if delta > a.lastDuration {
			a.lastDuration = delta
		}
		a.last = done
	}

	return nil
}

func (a *display) Close() error {
	return a.prompter.Close()
}

func (a *display) Finished(text string) {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	a.stopped = true
	a.area.Finish(text)
}
