package main

import (
	"io"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

func newRootCommand(version string, stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "diskseek",
		Short: "A disk-backed search engine",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.AddCommand(newVersionCommand(version))
	return command
}

func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the DiskSeek version",
		Args:  cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			command.Printf("version=%s\n", version)
		},
	}
}

func execute(args []string, stdout, stderr io.Writer, version string) int {
	command := newRootCommand(version, stdout, stderr)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		return 1
	}
	return 0
}
