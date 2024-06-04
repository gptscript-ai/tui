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

	if err := tui.Run(ctx, os.Args[1], tui.RunOptions{
		Workspace:           "./workspace",
		TrustedRepoPrefixes: []string{"github.com/gptscript-ai/context"},
		DisableCache:        true,
	}); err != nil {
		log.Fatal(err)
	}

	// This will fail if there are files, which is the desired behavior
	_ = os.Remove("./workspace")
}
