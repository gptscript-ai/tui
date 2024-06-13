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
)

type prompter struct {
	prompt    string
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

	return &prompter{
		readliner: l,
	}, nil
}

func (r *prompter) ReadPassword() (string, bool, error) {
	cfg := r.readliner.GenPasswordConfig()
	cfg.MaskRune = '*'
	cfg.Prompt = r.prompt
	cfg.UniqueEditLine = true
	line, err := r.readliner.ReadPasswordWithConfig(cfg)
	if errors.Is(err, readline.ErrInterrupt) {
		return "", false, nil
	} else if errors.Is(err, io.EOF) {
		return "", false, nil
	}
	return strings.TrimSpace(string(line)), true, nil
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

func (r *prompter) SetPrompt(text string) {
	r.prompt = color.GreenString(text+">") + " "
	r.readliner.SetPrompt(r.prompt)
}

func (r *prompter) Close() error {
	return r.readliner.Close()
}
