package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/gptscript-ai/go-gptscript"
	godiffpatch "github.com/sourcegraph/go-diff-patch"
)

type Confirm struct {
	trustedMap      map[string]struct{}
	always          []Trusted
	client          *gptscript.GPTScript
	authFile        string
	trustedPrefixes []string
}

func NewConfirm(appName string, client *gptscript.GPTScript, trustedRepoPrefixes ...string) (*Confirm, error) {
	authFile, err := xdg.CacheFile(fmt.Sprintf("%s/authorized.json", appName))
	if err != nil {
		return nil, err
	}

	c := &Confirm{
		trustedMap:      map[string]struct{}{},
		trustedPrefixes: trustedRepoPrefixes,
		client:          client,
		authFile:        authFile,
	}

	data, err := os.ReadFile(authFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Don't care if it fails
	_ = json.Unmarshal(data, &c.trustedMap)

	return c, nil
}

func (c *Confirm) HandlePrompt(ctx context.Context, event gptscript.Frame, prompter func(string, bool) (string, bool)) (bool, error) {
	if !c.IsPromptEvent(event) {
		return true, nil
	}

	values := map[string]string{}

	for i, field := range event.Prompt.Fields {
		msg := field
		if i == 0 {
			if len(event.Prompt.Fields) == 1 {
				msg = ""
			}
			msg = event.Prompt.Message + "\n" + msg
		}

		v, ok := prompter(msg, event.Prompt.Sensitive)
		if !ok {
			return ok, nil
		}
		values[field] = v
	}

	return true, c.client.PromptResponse(ctx, gptscript.PromptResponse{
		ID:        event.Prompt.ID,
		Responses: values,
	})
}

func (c *Confirm) HandleConfirm(ctx context.Context, event gptscript.Frame, prompter func(string) (Answer, bool, error)) (bool, error) {
	if !c.IsConfirmEvent(event) {
		return true, nil
	}

	prompt, trusted, err := c.IsTrusted(event)
	if err != nil {
		return true, err
	}

	var (
		reason string
		answer Answer
		ok     bool
	)

	if !trusted {
		answer, ok, err = prompter(prompt.Message)
		if !ok || err != nil {
			return ok, err
		}
		if answer == No {
			reason = "User rejected action, abort the current operation and ask the user how to proceed"
		} else {
			trusted = true
			c.SetTrusted(prompt, answer)
		}
	}

	return true, c.client.Confirm(ctx, gptscript.AuthResponse{
		ID:      event.Call.ID,
		Accept:  trusted,
		Message: reason,
	})
}

func (c *Confirm) SetTrusted(prompt ConfirmPrompt, answer Answer) {
	if answer == No {
		return
	}

	repo := prompt.Repo
	if _, ok := c.trustedMap[repo]; repo != "" && !ok {
		c.trustedMap[repo] = struct{}{}
		data, err := json.Marshal(c.trustedMap)
		if err != nil {
			return
		}
		_ = os.WriteFile(c.authFile, data, 0600)
	}

	if answer == Always && prompt.AlwaysTrust.ToolName != "" {
		c.always = append(c.always, prompt.AlwaysTrust)
	}
}

func (c *Confirm) IsConfirmEvent(event gptscript.Frame) bool {
	return event.Call != nil && event.Call.Type == gptscript.EventTypeCallConfirm
}

func (c *Confirm) IsPromptEvent(event gptscript.Frame) bool {
	return event.Prompt != nil && event.Prompt.Type == gptscript.EventTypePrompt
}

func (c *Confirm) getRepo(event gptscript.Frame) string {
	if event.Call.Tool.Source.Repo != nil {
		if strings.HasPrefix(event.Call.Tool.Source.Repo.Root, "https://github.com/") {
			repo := strings.TrimPrefix(event.Call.Tool.Source.Repo.Root, "https://")
			return strings.TrimSuffix(repo, ".git")
		}
		return event.Call.Tool.Source.Repo.Root
	}
	return ""
}

func (c *Confirm) isAlways(event gptscript.Frame) bool {
	sysToolName, isSysTool := isSysTool(event, "")
	if !isSysTool {
		return false
	}

	for _, trusted := range c.always {
		if trusted.ToolName == sysToolName {
			if len(c.trustedPrefixes) == 0 {
				return true
			}
			args := inputArgs(event)
			for name, prefix := range trusted.ArgPrefix {
				val, _ := args[name].(string)
				if !strings.HasPrefix(val, prefix) {
					return false
				}
			}
			return true
		}
	}

	return false
}

func (c *Confirm) IsTrusted(event gptscript.Frame) (ConfirmPrompt, bool, error) {
	repo := c.getRepo(event)
	if _, ok := c.trustedMap[repo]; repo != "" && ok {
		return ConfirmPrompt{}, true, nil
	}

	for _, prefix := range c.trustedPrefixes {
		if repo == prefix || strings.HasPrefix(repo, prefix+"/") {
			return ConfirmPrompt{}, true, nil
		}
	}

	if repo != "" {
		return ConfirmPrompt{
			Message: fmt.Sprintf("Do you trust tools from the git repository [%s] (y/n)", repo),
			Repo:    repo,
		}, false, nil
	}

	if c.isAlways(event) {
		return ConfirmPrompt{}, true, nil
	}

	if sysToolName, isSysTool := isSysTool(event, ""); isSysTool {
		return toSysConfirmMessage(sysToolName, event), false, nil
	}

	return ConfirmPrompt{}, true, nil
}

func isSysTool(event gptscript.Frame, sysName string) (string, bool) {
	return strings.TrimPrefix(event.Call.Tool.Instructions[2:], "sys."),
		strings.HasPrefix(event.Call.Tool.Instructions, "#!sys."+sysName)
}

func inputArgs(event gptscript.Frame) map[string]any {
	data := map[string]any{}
	_ = json.Unmarshal([]byte(event.Call.Input), &data)
	return data
}

type ConfirmPrompt struct {
	Repo        string
	Message     string
	AlwaysTrust Trusted
}

type Trusted struct {
	ToolName  string
	ArgPrefix map[string]string
}

func toSysConfirmMessage(toolName string, event gptscript.Frame) (prompt ConfirmPrompt) {
	var ok bool

	switch toolName {
	case "write":
		prompt, ok = toWritePrompt(event)
	case "exec":
		prompt, ok = toExecPrompt(event)
	}
	if ok {
		return
	}

	text := toolName
	if event.Call.DisplayText != "" {
		text = strings.ToLower(event.Call.DisplayText[:1]) + event.Call.DisplayText[1:]
	}

	return ConfirmPrompt{
		Message: fmt.Sprintf("Proceed with %s (or allow all %s calls)\nConfirm (y/n/a)", text, toolName),
		AlwaysTrust: Trusted{
			ToolName: toolName,
		},
	}
}

func toExecPrompt(event gptscript.Frame) (ConfirmPrompt, bool) {
	data := inputArgs(event)
	command, _ := data["command"].(string)
	directory, _ := data["directory"].(string)
	if command == "" {
		return ConfirmPrompt{}, false
	}
	msg := &strings.Builder{}
	msg.WriteString("Run \"")
	msg.WriteString(command)
	msg.WriteString("\"")
	if directory != "" {
		msg.WriteString(" in directory ")
		msg.WriteString(directory)
	}

	parts := strings.Fields(command)
	prefix := parts[0]
	if len(parts) > 1 && !strings.HasPrefix(parts[1], "-") && !strings.Contains(parts[1], ".") {
		prefix += " " + parts[1]
	}

	msg.WriteString(" (or allow all \"")
	msg.WriteString(prefix)
	msg.WriteString(" ...\" commands)\nConfirm (y/n/a)")

	return ConfirmPrompt{
		Message: msg.String(),
		AlwaysTrust: Trusted{
			ToolName: "exec",
			ArgPrefix: map[string]string{
				"command": prefix,
			},
		},
	}, true
}

func toWritePrompt(event gptscript.Frame) (ConfirmPrompt, bool) {
	data := inputArgs(event)
	filename, _ := data["filename"].(string)
	content, _ := data["content"].(string)
	if filename == "" || content == "" {
		return ConfirmPrompt{}, false
	}

	existing, err := os.ReadFile(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return ConfirmPrompt{
			Message: fmt.Sprintf("%s\nWrite to %s \nConfirm (y/n)",
				markdownBox("", content), filename),
		}, true
	} else if err == nil {
		patch := godiffpatch.GeneratePatch(filepath.Base(filename), string(existing), content)
		return ConfirmPrompt{
			Message: fmt.Sprintf("%s\nUpdate %s\nConfirm (y/n)",
				markdownBox("diff", patch), filename),
		}, true
	}

	return ConfirmPrompt{}, false
}
