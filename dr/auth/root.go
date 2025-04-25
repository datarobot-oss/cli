package auth

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/datarobot/cli/dr"
)

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "DataRobot authentication commands",
	Long:  `Authentication commands for DataRobot CLI.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := AuthCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	dr.RootCmd.AddCommand(AuthCmd)
}
