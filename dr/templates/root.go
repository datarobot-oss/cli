package templates

import (
	"os"

	"github.com/datarobot/cli/dr"
	"github.com/spf13/cobra"
)

var TemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "DataRobot application templates commands",
	Long:  `Application templates commands for DataRobot CLI.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := TemplatesCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	dr.RootCmd.AddCommand(TemplatesCmd)
}
