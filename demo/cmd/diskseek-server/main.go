package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Exclearf/diskseek/demo/internal/httpapi"
	"github.com/Exclearf/diskseek/demo/internal/webui"
	"github.com/spf13/cobra"
)

const defaultListenAddress = "127.0.0.1:8080"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	command := newCommand()
	command.SetContext(ctx)
	if err := command.Execute(); err != nil {
		stop()
		os.Exit(1)
	}
	stop()
}

func newCommand() *cobra.Command {
	listenAddress := defaultListenAddress
	datasetPath := ""
	command := &cobra.Command{
		Use:   "diskseek-server",
		Short: "Serve DiskSeek indexes over HTTP",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (err error) {
			if datasetPath == "" {
				return errors.New("dataset is required")
			}
			frontend, err := webui.New()
			if err != nil {
				return err
			}

			dataset, err := httpapi.OpenDataset(datasetPath)
			if err != nil {
				return fmt.Errorf("open dataset: %w", err)
			}
			defer func() { err = errors.Join(err, dataset.Index.Close()) }()

			listener, err := net.Listen("tcp", listenAddress)
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}
			handler := http.NewServeMux()
			handler.Handle("/v1/", httpapi.New(dataset))
			handler.Handle("/", frontend)
			server := http.Server{Handler: handler}
			stop := context.AfterFunc(command.Context(), func() {
				_ = server.Close()
			})
			defer stop()

			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("serve: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&listenAddress, "listen", listenAddress, "HTTP listen address")
	command.Flags().StringVar(&datasetPath, "dataset", datasetPath, "dataset bundle")
	return command
}
