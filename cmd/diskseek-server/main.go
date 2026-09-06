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

	"github.com/Exclearf/diskseek/internal/httpapi"
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
	datasetPaths := make(map[string]string)
	command := &cobra.Command{
		Use:   "diskseek-server",
		Short: "Serve DiskSeek indexes over HTTP",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (err error) {
			if len(datasetPaths) == 0 {
				return errors.New("at least one dataset is required")
			}

			datasets := make(map[string]httpapi.Dataset, len(datasetPaths))
			defer func() {
				for _, dataset := range datasets {
					err = errors.Join(err, dataset.Index.Close())
				}
			}()
			for name, path := range datasetPaths {
				dataset, err := httpapi.OpenDataset(path)
				if err != nil {
					return fmt.Errorf("open dataset %q: %w", name, err)
				}
				datasets[name] = dataset
			}

			listener, err := net.Listen("tcp", listenAddress)
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}
			server := http.Server{Handler: httpapi.New(datasets)}
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
	command.Flags().StringToStringVar(&datasetPaths, "dataset", datasetPaths, "dataset as ID=BUNDLE")
	return command
}
