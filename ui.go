package tui

import (
	"context"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gptscript-ai/go-gptscript"
	"golang.org/x/exp/maps"
)

func toStringCall(calls map[string]gptscript.CallFrame, call gptscript.CallFrame) string {
	buf := strings.Builder{}

	if call.DisplayText != "" {
		s, _ := MarkdownRender.Render(call.DisplayText)
		buf.WriteString(s)
	}

	for _, output := range call.Output {
		if content, _, _ := strings.Cut(output.Content, "<tool call>"); content != "" {
			if strings.HasPrefix(call.Tool.Instructions, "#!") {
				buf.WriteString(BoxStyle.Render(strings.TrimSpace(content)))
			} else {
				s, err := MarkdownRender.Render(content)
				if err == nil {
					buf.WriteString(s)
				} else {
					buf.WriteString(content)
				}
			}
		}

		keys := maps.Keys(output.SubCalls)
		sort.Slice(keys, func(i, j int) bool {
			return calls[keys[i]].Start.Before(calls[keys[j]].Start)
		})

		for _, key := range keys {
			buf.WriteString(toStringCall(calls, calls[key]))
		}
	}

	return buf.String()
}

func toString(input string, calls map[string]gptscript.CallFrame) (result string) {
	if input != "" {
		result = color.GreenString("> "+input) + "\n"
	}

	for _, call := range calls {
		if call.ParentID == "" {
			return result + toStringCall(calls, call)
		}
	}

	return result
}

func Run(ctx context.Context, tool, workspace, input string) error {
	prompt, err := newReadlinePrompter(tool)
	if err != nil {
		return err
	}
	defer prompt.Close()

	client, err := gptscript.NewClient()
	if err != nil {
		return err
	}
	defer client.Close()

	run, err := client.Run(ctx, tool, gptscript.Options{
		Confirm:       true,
		IncludeEvents: true,
		Workspace:     workspace,
		Input:         input,
	})
	if err != nil {
		return err
	}

	for {
		var (
			prg   gptscript.Program
			calls = map[string]gptscript.CallFrame{}
			text  string
		)

		area, err := newAreaPrinter()
		if err != nil {
			return err
		}

		for event := range run.Events() {
			if event.Run != nil {
				prg = event.Run.Program
			}

			if event.Call != nil {
				calls[event.Call.ID] = *event.Call
			}

			text = toString(input, calls)
			area.Update(text)

			if event.Call != nil && event.Call.Type == gptscript.EventTypeCallConfirm {
				err := client.Confirm(ctx, gptscript.AuthResponse{
					ID:     event.Call.ID,
					Accept: true,
				})
				if err != nil {
					return err
				}
			}
		}

		err = area.Stop(text)
		if err != nil {
			return err
		}

		if run.State().IsTerminal() {
			_ = run.Close()
			return run.Err()
		}

		prompt.SetPrompt(run, prg)

		line, ok, err := prompt.Readline()
		if err != nil || !ok {
			return err
		}

		input = line

		run, err = run.NextChat(ctx, line)
		if err != nil {
			return err
		}
	}
}
