package cmd

import (
	"os"

	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/go-common/env"
	csys "github.com/agentuity/go-common/sys"
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
		logger := env.NewLogger(cmd)
		dir := resolveProjectDir(logger, cmd)
		apiUrl := viper.GetString("overrides.api_url")
		provider, err := provider.RunDev(logger, dir, apiUrl, args)
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
