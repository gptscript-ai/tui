package tui

import (
	"context"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gptscript-ai/go-gptscript"
	"golang.org/x/exp/maps"
)

type RunOptions struct {
	TrustedRepoPrefixes []string
}

func complete(opts ...RunOptions) (result RunOptions) {
	for _, opt := range opts {
		result.TrustedRepoPrefixes = append(result.TrustedRepoPrefixes, opt.TrustedRepoPrefixes...)
	}
	return
}

func Run(ctx context.Context, tool, workspace, input string, opts ...RunOptions) error {
	opt := complete(opts...)

	client, err := gptscript.NewClient()
	if err != nil {
		return err
	}
	defer client.Close()

	confirm, err := NewConfirm(tool, client, opt.TrustedRepoPrefixes...)
	if err != nil {
		return err
	}

	ui, err := newDisplay(tool)
	if err != nil {
		return err
	}
	defer ui.Close()

	run, err := client.Run(ctx, tool, gptscript.Options{
		Confirm:       true,
		IncludeEvents: true,
		Workspace:     workspace,
		Input:         input,
	})
	if err != nil {
		return err
	}
	defer run.Close()

	for {
		var (
			prg   gptscript.Program
			calls = map[string]gptscript.CallFrame{}
			text  string
		)

		for event := range run.Events() {
			if event.Run != nil {
				prg = event.Run.Program
			}

			if event.Call != nil {
				calls[event.Call.ID] = *event.Call
			}

			text = render(input, calls)
			if err := ui.Progress(text); err != nil {
				return err
			}

			if ok, err := confirm.HandlePrompt(ctx, event, ui.Ask); !ok || err != nil {
				return err
			}

			if ok, err := confirm.HandleConfirm(ctx, event, ui.AskYesNo); !ok || err != nil {
				return err
			}
		}

		err = ui.Finished(text)
		if err != nil {
			return err
		}

		if run.State().IsTerminal() {
			return run.Err()
		}

		line, ok, err := ui.Prompt(getCurrentToolName(prg, run))
		if err != nil || !ok {
			return err
		}

		input = line
		run, err = run.NextChat(ctx, input)
		if err != nil {
			return err
		}
	}
}

func render(input string, calls map[string]gptscript.CallFrame) string {
	buf := &strings.Builder{}

	if input != "" {
		buf.WriteString(color.GreenString("> "+input) + "\n")
	}

	for _, call := range calls {
		if call.ParentID == "" {
			printCall(buf, calls, call)
		}
	}

	return buf.String()
}

func printCall(buf *strings.Builder, calls map[string]gptscript.CallFrame, call gptscript.CallFrame) {
	if call.DisplayText != "" {
		s, err := MarkdownRender.Render(call.DisplayText)
		if err == nil {
			buf.WriteString(s)
		}
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
			printCall(buf, calls, calls[key])
		}
	}
}

func getCurrentToolName(prg gptscript.Program, run *gptscript.Run) string {
	out, _ := run.RawOutput()
	toolID, _ := out["toolID"].(string)
	toolName := prg.ToolSet[toolID].Name
	if toolName == "" {
		return toolName
	}
	return "@" + toolName
}
