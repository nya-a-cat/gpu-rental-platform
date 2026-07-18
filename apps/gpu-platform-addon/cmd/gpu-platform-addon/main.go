package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	addonagent "github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/addonagent"
	addonmanager "github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/addonmanager"
	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/options"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		log.Printf("gpu platform addon failed: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("a subcommand is required: manager, controller, or agent")
	}

	switch args[0] {
	case "manager", "controller":
		opts, err := options.ParseManager(args[1:])
		if err != nil {
			return fmt.Errorf("parse manager options: %w", err)
		}
		return addonmanager.Run(ctx, opts)
	case "agent":
		opts, err := options.ParseAgent(args[1:])
		if err != nil {
			return fmt.Errorf("parse agent options: %w", err)
		}
		return addonagent.Run(ctx, opts)
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stdout, "usage: gpu-platform-addon <manager|controller|agent> [flags]")
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q; expected manager, controller, or agent", args[0])
	}
}
