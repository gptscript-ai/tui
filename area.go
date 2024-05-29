package tui

import (
	"strings"
	"time"

	"github.com/pterm/pterm"
)

type areaPrinter struct {
	area *pterm.AreaPrinter
	last time.Time
}

func newAreaPrinter() (*areaPrinter, error) {
	area, err := pterm.DefaultArea.Start()
	if err != nil {
		return nil, err
	}

	return &areaPrinter{
		area: area,
	}, nil
}

func (a *areaPrinter) Update(text string) {
	now := time.Now()
	if now.Sub(a.last).Milliseconds() > 200 {
		if a.last.IsZero() {
			a.last = now.Add(-time.Second)
		} else {
			a.last = now
		}
		lines := strings.Split(text, "\n")
		height := pterm.GetTerminalHeight()
		if len(lines) > height {
			lines = lines[len(lines)-height:]
		}
		a.area.Update(strings.Join(lines, "\n"))
	}
}

func (a *areaPrinter) Stop(text string) error {
	a.area.Update(text)
	return a.area.Stop()
}
