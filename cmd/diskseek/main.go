package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	status := execute(ctx, os.Args[1:], os.Stdout, os.Stderr, version)
	stop()
	os.Exit(status)
}
