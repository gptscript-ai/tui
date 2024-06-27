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
	always          map[string]struct{}
	client          gptscript.GPTScript
	authFile        string
	trustedPrefixes []string
}

func NewConfirm(appName string, client gptscript.GPTScript, trustedRepoPrefixes ...string) (*Confirm, error) {
	authFile, err := xdg.CacheFile(fmt.Sprintf("%s/authorized.json", appName))
	if err != nil {
		return nil, err
	}

	c := &Confirm{
		trustedMap:      map[string]struct{}{},
		trustedPrefixes: trustedRepoPrefixes,
		always:          map[string]struct{}{},
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

func (c *Confirm) HandlePrompt(ctx context.Context, event gptscript.Frame, prompter func(string, bool) (string, bool, error)) (bool, error) {
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

		v, ok, err := prompter(msg, event.Prompt.Sensitive)
		if !ok || err != nil {
			return ok, err
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

	msg, trusted, err := c.IsTrusted(event)
	if err != nil {
		return true, err
	}

	var (
		reason string
		answer Answer
		ok     bool
	)

	if !trusted {
		answer, ok, err = prompter(msg)
		if !ok || err != nil {
			return ok, err
		}
		if answer == No {
			reason = "User rejected action, abort operation and ask how to proceed"
		} else {
			trusted = true
			c.SetTrusted(event, answer)
		}
	}

	return true, c.client.Confirm(ctx, gptscript.AuthResponse{
		ID:      event.Call.ID,
		Accept:  trusted,
		Message: reason,
	})
}

func (c *Confirm) SetTrusted(event gptscript.Frame, answer Answer) {
	if answer == No {
		return
	}

	repo := c.getRepo(event)
	if _, ok := c.trustedMap[repo]; repo != "" && !ok {
		c.trustedMap[repo] = struct{}{}
		data, err := json.Marshal(c.trustedMap)
		if err != nil {
			return
		}
		_ = os.WriteFile(c.authFile, data, 0600)
	}

	if answer == Always && strings.HasPrefix(event.Call.Tool.Instructions, "#!sys.") {
		c.always[event.Call.Tool.Instructions] = struct{}{}
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

func (c *Confirm) IsTrusted(event gptscript.Frame) (string, bool, error) {
	repo := c.getRepo(event)
	if _, ok := c.trustedMap[repo]; repo != "" && ok {
		return "", true, nil
	}

	for _, prefix := range c.trustedPrefixes {
		if repo == prefix || strings.HasPrefix(repo, prefix+"/") {
			return "", true, nil
		}
	}

	if repo != "" {
		return fmt.Sprintf("Do you trust tools from the git repository [%s] (y/n)", repo), false, nil
	}

	if _, isSysTool := isSysTool(event, ""); isSysTool && event.Call.DisplayText != "" {
		if _, ok := c.always[event.Call.Tool.Instructions]; ok {
			return "", true, nil
		}
		return toConfirmMessage(event), false, nil
	}

	return "", true, nil
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

func toConfirmMessage(event gptscript.Frame) string {
	if tool, ok := isSysTool(event, "write"); ok {
		data := inputArgs(event)
		filename, _ := data["filename"].(string)
		content, _ := data["content"].(string)
		if filename != "" && content != "" {
			existing, err := os.ReadFile(filename)
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Sprintf("%s\nWrite to %s (or allow all %s calls)\nConfirm (y/n/a)",
					markdownBox("", content), filename, tool)
			} else if err == nil {
				patch := godiffpatch.GeneratePatch(filepath.Base(filename), string(existing), content)
				return fmt.Sprintf("%s\nUpdate %s (or allow all %s calls)\nConfirm (y/n/a)",
					markdownBox("diff", patch), filename, tool)
			}
		}
	}
	return fmt.Sprintf("Proceed with %s (or allow all %s calls)\nConfirm (y/n/a)",
		strings.ToLower(event.Call.DisplayText[:1])+event.Call.DisplayText[1:],
		strings.TrimPrefix(event.Call.Tool.Instructions[2:], "sys."))
}
