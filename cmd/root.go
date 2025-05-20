package cmd

import (
	_ "github.com/datarobot/cli/cmd/auth"
	_ "github.com/datarobot/cli/cmd/templates"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   version.AppName,
	Short: "The DataRobot CLI",
	Long: `
	The DataRobot CLI is a command-line interface for interacting with
	DataRobot's application templates and authentication. It allows users to 
	clone, configure, and deploy applications to their DataRobot production environment.
	`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return RootCmd.Execute()
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.golang-cobra.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	RootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
