package tui

import (
	"strings"
	"time"

	"github.com/pterm/pterm"
)

type display struct {
	area     *pterm.AreaPrinter
	prompter *prompter
	last     time.Time
	stopped  bool
}

func newDisplay(tool string) (*display, error) {
	area, err := pterm.DefaultArea.Start()
	if err != nil {
		return nil, err
	}

	prompter, err := newReadlinePrompter(tool)
	if err != nil {
		return nil, err
	}

	return &display{
		area:     area,
		prompter: prompter,
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
		a.area.Update(a.area.GetContent() + "\n" + strings.Join(lines[:len(lines)-1], "\n") + "\n")
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
	if a.stopped {
		area, err := pterm.DefaultArea.Start()
		if err != nil {
			return err
		}
		a.area = area
		a.stopped = false
		a.last = time.Time{}
	}

	now := time.Now()
	if now.Sub(a.last).Milliseconds() > 50 {
		a.last = now
		lines := strings.Split(text, "\n")
		height := pterm.GetTerminalHeight()
		if len(lines) > height {
			lines = lines[len(lines)-height:]
		}
		a.area.Update(strings.Join(lines, "\n"))
	}

	return nil
}

func (a *display) Close() error {
	return a.prompter.Close()
}

func (a *display) Finished(text string) error {
	a.stopped = true
	a.area.Update(text)
	return a.area.Stop()
}
