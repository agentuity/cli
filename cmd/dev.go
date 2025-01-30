package cmd

import (
	"os"

	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/project"
	csys "github.com/shopmonkeyus/go-common/sys"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var devRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the development server",
	Run: func(cmd *cobra.Command, args []string) {
		logger := newLogger(cmd)
		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatal("failed to get current directory: %s", err)
		}
		dir := cwd
		dirFlag, _ := cmd.Flags().GetString("dir")
		if dirFlag != "" {
			dir = dirFlag
		}
		if !project.ProjectExists(dir) {
			logger.Fatal("no agentuity.yaml file found in the current directory")
		}
		apiUrl := viper.GetString("overrides.api_url")
		provider, err := dev.NewProvider(logger, dir, args, apiUrl)
		if err != nil {
			logger.Fatal("failed to run development server: %s", err)
		}
		if err := provider.Start(); err != nil {
			logger.Fatal("failed to start development server: %s", err)
		}
		// TODO: hook up watch
		for {
			select {
			case <-provider.Done():
				logger.Info("development server stopped")
				os.Exit(0)
			case <-provider.Restart():
				if err := provider.Stop(); err != nil {
					logger.Warn("failed to stop development server: %s", err)
				}
				if err := provider.Start(); err != nil {
					logger.Fatal("failed to restart development server: %s", err)
				}
			case <-csys.CreateShutdownChannel():
				if err := provider.Stop(); err != nil {
					logger.Warn("failed to stop development server: %s", err)
				}
				return
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devRunCmd)
	devRunCmd.Flags().StringP("dir", "d", ".", "The directory to run the development server in")
}
