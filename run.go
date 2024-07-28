package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/pterm/pterm"
	"golang.org/x/exp/maps"
)

const ToolCallHeader = "<tool call>"

type RunOptions struct {
	AppName               string
	Eval                  []gptscript.ToolDef
	TrustedRepoPrefixes   []string
	DisableCache          bool
	CredentialOverrides   []string
	Input                 string
	CacheDir              string
	SubTool               string
	ChatState             string
	SaveChatStateFile     string
	Workspace             string
	UserStartConversation *bool
	Location              string
	EventLog              string
	LoadMessage           string
	Client                *gptscript.GPTScript
	ClientOpts            *gptscript.GlobalOptions

	deleteWorkspaceOn bool
}

func first[T comparable](in ...T) (result T) {
	for _, i := range in {
		if i != result {
			return i
		}
	}
	return
}

func complete(opts ...RunOptions) (result RunOptions, closeClient func(), _ error) {
	closeClient = func() {}

	for _, opt := range opts {
		result.TrustedRepoPrefixes = append(result.TrustedRepoPrefixes, opt.TrustedRepoPrefixes...)
		result.DisableCache = first(opt.DisableCache, result.DisableCache)
		result.CredentialOverrides = append(result.CredentialOverrides, opt.CredentialOverrides...)
		result.CacheDir = first(opt.CacheDir, result.CacheDir)
		result.SubTool = first(opt.SubTool, result.SubTool)
		result.Workspace = first(opt.Workspace, result.Workspace)
		result.SaveChatStateFile = first(opt.SaveChatStateFile, result.SaveChatStateFile)
		result.ChatState = first(opt.ChatState, result.ChatState)
		result.Eval = append(result.Eval, opt.Eval...)
		result.AppName = first(opt.AppName, result.AppName)
		result.UserStartConversation = first(opt.UserStartConversation, result.UserStartConversation)
		result.Location = first(opt.Location, result.Location)
		result.EventLog = first(opt.EventLog, result.EventLog)
		result.LoadMessage = first(opt.LoadMessage, result.LoadMessage)
		result.Client = first(opt.Client, result.Client)
		result.ClientOpts = first(opt.ClientOpts, result.ClientOpts)
	}
	if result.AppName == "" {
		result.AppName = "gptscript-tui"
	}

	if result.Workspace == "" {
		var err error
		result.Workspace, err = os.MkdirTemp("", fmt.Sprintf("%s-workspace-*", result.AppName))
		if err != nil {
			return result, closeClient, err
		}
		result.deleteWorkspaceOn = true
	} else if !filepath.IsAbs(result.Workspace) {
		var err error
		result.Workspace, err = filepath.Abs(result.Workspace)
		if err != nil {
			return result, closeClient, err
		}
	}

	if err := os.MkdirAll(result.Workspace, 0700); err != nil {
		return result, closeClient, err
	}

	if result.Client != nil && result.ClientOpts != nil {
		return result, closeClient, fmt.Errorf("only one of Client or ClientOpts may be set, not both")
	}

	if result.Client == nil {
		var (
			opts gptscript.GlobalOptions
			err  error
		)
		if result.ClientOpts != nil {
			opts = *result.ClientOpts
		}
		result.Client, err = gptscript.NewGPTScript(opts)
		if err != nil {
			return result, closeClient, err
		}
		closeClient = result.Client.Close
	}

	return
}

func Run(ctx context.Context, tool string, opts ...RunOptions) error {
	var (
		opt, closeClient, err = complete(opts...)
		input                 = opt.Input
		localCtx, cancel      = signal.NotifyContext(ctx, os.Interrupt)
		eventOut              io.Writer
	)
	defer cancel()

	if err != nil {
		return err
	}
	defer closeClient()
	if opt.deleteWorkspaceOn {
		defer os.RemoveAll(opt.Workspace)
	}

	startCtx, started := context.WithCancel(ctx)
	defer started()
	go func() {
		select {
		case <-startCtx.Done():
			return
		case <-time.After(time.Second):
		}

		if opt.LoadMessage != "" {
			fmt.Printf(opt.LoadMessage)
		}
	}()

	client := opt.Client
	confirm, err := NewConfirm(opt.AppName, client, opt.TrustedRepoPrefixes...)
	if err != nil {
		return err
	}

	ui, err := newDisplay(tool)
	if err != nil {
		return err
	}
	defer ui.Close()

	if opt.UserStartConversation == nil {
		tools := opt.Eval
		if len(tools) == 0 {
			nodes, err := client.Parse(ctx, tool)
			if err != nil {
				return err
			}
			for _, node := range nodes {
				if node.ToolNode != nil {
					tools = append(tools, node.ToolNode.Tool.ToolDef)
				}
			}
		}
		if len(tools) > 0 {
			if tools[0].Chat && tools[0].Instructions == "" {
				opt.UserStartConversation = &[]bool{true}[0]
			}
		}
	}

	firstInput := opt.Input

	if firstInput == "" && opt.UserStartConversation != nil && *opt.UserStartConversation {
		var ok bool
		firstInput, ok = ui.Prompt("")
		if !ok {
			return nil
		}
	}

	if firstInput == "" && opt.ChatState != "" {
		var ok bool
		firstInput, ok = ui.Prompt("Resuming conversation")
		if !ok {
			return nil
		}
	}

	var run *gptscript.Run
	runOpt := gptscript.Options{
		Confirm:             true,
		Prompt:              true,
		IncludeEvents:       true,
		DisableCache:        opt.DisableCache,
		CredentialOverrides: opt.CredentialOverrides,
		Input:               firstInput,
		CacheDir:            opt.CacheDir,
		SubTool:             opt.SubTool,
		Workspace:           opt.Workspace,
		ChatState:           opt.ChatState,
		Location:            opt.Location,
	}
	if len(opt.Eval) == 0 {
		run, err = client.Run(localCtx, tool, runOpt)
		if err != nil {
			return err
		}
		defer run.Close()
	} else {
		run, err = client.Evaluate(localCtx, runOpt, opt.Eval...)
		if err != nil {
			return err
		}
		defer run.Close()
	}

	if opt.EventLog != "" {
		f, err := os.OpenFile(opt.EventLog, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		eventOut = f
	}

	for {
		var text string

		for event := range run.Events() {
			started()
			if eventOut != nil && (event.Call == nil || event.Call.Type != gptscript.EventTypeCallProgress) {
				if err := json.NewEncoder(eventOut).Encode(map[string]any{
					"time":  time.Now(),
					"event": event,
				}); err != nil {
					return err
				}
			}

			if event.Call != nil {
				text = render(input, run)
				if err := ui.Progress(text); err != nil {
					return err
				}
			}

			if ok, err := confirm.HandlePrompt(localCtx, event, ui.Ask); !ok {
				return nil
			} else if err != nil && localCtx.Err() == nil {
				return err
			}

			if ok, err := confirm.HandleConfirm(localCtx, event, ui.AskYesNo); !ok {
				return nil
			} else if err != nil && localCtx.Err() == nil {
				return err
			}
		}

		if errors.Is(localCtx.Err(), context.Canceled) {
			text = "Interrupted\n\n"
		}

		ui.Finished(text)

		if opt.SaveChatStateFile != "" {
			if run.State() == gptscript.Finished {
				_ = os.Remove(opt.SaveChatStateFile)
			} else {
				_ = os.WriteFile(opt.SaveChatStateFile, []byte(run.ChatState()), 0600)
			}
		}

		for {
			if run != nil && run.State().IsTerminal() {
				if errors.Is(localCtx.Err(), context.Canceled) {
				} else if run.Err() != nil {
					fmt.Println(color.RedString("%v", run.Err()))
				} else {
					return nil
				}
			}

			// reset interrupt
			cancel()
			localCtx, cancel = signal.NotifyContext(ctx, os.Interrupt)

			line, ok := ui.Prompt(getCurrentToolName(run))
			if !ok {
				return nil
			}

			input = line
			run, err = run.NextChat(localCtx, input)
			if err != nil {
				fmt.Println(color.RedString("%v", run.Err()))
				run = nil
				continue
			}

			break
		}
	}
}

func render(input string, run *gptscript.Run) string {
	buf := &strings.Builder{}

	if input != "" {
		buf.WriteString(color.GreenString("> "+input) + "\n")
	}

	if call, ok := run.ParentCallFrame(); ok {
		printCall(buf, run.Calls(), call, nil)
	}

	return buf.String()
}

func printToolCall(out *strings.Builder, toolCall string) {
	// The intention here is to only print the string while it's still being generated, if it's complete
	// then there's no reason to because we are waiting on something else at that point and it's status should
	// be displayed
	lines := strings.Split(toolCall, "\n")
	buf := &strings.Builder{}

	for _, line := range lines {
		name, args, ok := strings.Cut(strings.TrimPrefix(line, ToolCallHeader), " -> ")
		if !ok {
			continue
		}
		width := pterm.GetTerminalWidth() - 33
		if len(args) > width {
			args = fmt.Sprintf("%s %s...(%d)", name, args[:width], len(args[width:]))
		} else {
			args = fmt.Sprintf("%s %s", name, args)
		}

		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(strings.TrimSpace(args))
	}

	if buf.Len() > 0 {
		out.WriteString("\n")
		out.WriteString(BoxStyle.Render("Call Arguments:\n\n" + buf.String()))
	}
}

func printCall(buf *strings.Builder, calls map[string]gptscript.CallFrame, call gptscript.CallFrame, stack []string) {
	if slices.Contains(stack, call.ID) {
		return
	}

	if call.DisplayText != "" {
		s, err := MarkdownRender.Render(call.DisplayText)
		if err == nil {
			buf.WriteString(s)
		}
	}

	// Here we try to print the status of credential/context tools that are taking a while to do things.
	if len(call.Output) == 0 {
		for _, child := range calls {
			if child.ID == call.ID {
				continue
			}
			if child.ParentID == call.ID {
				if len(child.Output) > 0 && child.End.IsZero() && time.Since(child.Start) > 1000*time.Millisecond {
					printCall(buf, calls, child, append(stack, call.ID))
				}
			}
		}
	}

	for _, output := range call.Output {
		content, toolCall, _ := strings.Cut(output.Content, ToolCallHeader)
		if content != "" {
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

		if toolCall != "" {
			printToolCall(buf, ToolCallHeader+toolCall)
		}

		keys := maps.Keys(output.SubCalls)
		sort.Slice(keys, func(i, j int) bool {
			return calls[keys[i]].Start.Before(calls[keys[j]].Start)
		})

		for _, key := range keys {
			printCall(buf, calls, calls[key], append(stack, call.ID))
		}
	}
}

func getCurrentToolName(run *gptscript.Run) string {
	toolName := run.RespondingTool().Name
	if toolName == "" {
		return ""
	}
	return "@" + toolName
}
