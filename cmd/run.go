package cmd

import "github.com/spf13/cobra"

var TaskRunCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"r"},
	Short:   "Run an application template task",
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: implement the run command
	},
}

func init() {
	// TODO: add arguments
}
