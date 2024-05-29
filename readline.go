package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/adrg/xdg"
	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/gptscript-ai/go-gptscript"
)

type prompter struct {
	readliner *readline.Instance
}

func id(s string) string {
	d := sha256.New()
	d.Write([]byte(s))
	hash := d.Sum(nil)
	return hex.EncodeToString(hash[:])
}

func newReadlinePrompter(tool string) (*prompter, error) {
	historyFile, err := xdg.CacheFile(fmt.Sprintf("gptscript/tui/chat-%s.history", id(tool)))
	if err != nil {
		historyFile = ""
	}

	l, err := readline.NewEx(&readline.Config{
		Prompt:            color.GreenString("> "),
		HistoryFile:       historyFile,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		UniqueEditLine:    true,
	})
	if err != nil {
		return nil, err
	}

	l.CaptureExitSignal()

	return &prompter{
		readliner: l,
	}, nil
}

func (r *prompter) Readline() (string, bool, error) {
	line, err := r.readliner.Readline()
	if errors.Is(err, readline.ErrInterrupt) {
		return "", false, nil
	} else if errors.Is(err, io.EOF) {
		return "", false, nil
	}
	return strings.TrimSpace(line), true, nil
}

func (r *prompter) SetPrompt(run *gptscript.Run, prg gptscript.Program) {
	out, _ := run.RawOutput()
	toolID, _ := out["toolID"].(string)
	if name := prg.ToolSet[toolID].Name; name == "" {
		r.readliner.SetPrompt(color.GreenString(">") + " ")
	} else {
		r.readliner.SetPrompt(color.GreenString("Talking to: "+name+">") + " ")
	}
}

func (r *prompter) Close() error {
	return r.readliner.Close()
}
