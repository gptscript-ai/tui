package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/gptscript-ai/tui"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: " + os.Args[0] + " [TOOL NAME]")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var state string
	data, err := os.ReadFile("state.json")
	if err == nil {
		state = string(data)
	}

	if err := tui.Run(ctx, os.Args[1], tui.RunOptions{
		Workspace:           "./workspace",
		TrustedRepoPrefixes: []string{"github.com/gptscript-ai/context"},
		DisableCache:        true,
		SaveChatStateFile:   "state.json",
		ChatState:           state,
	}); err != nil {
		log.Fatal(err)
	}

	// This will fail if there are files, which is the desired behavior
	_ = os.Remove("./workspace")
}
