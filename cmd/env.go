package cmd

import (
	"fmt"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/env"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Environment related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}


var envSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set environment variables",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		dir := resolveProjectDir(logger, cmd)
		apiUrl := viper.GetString("overrides.api_url")
		apiKey := viper.GetString("auth.api_key")
		if apiKey == "" {
			logger.Fatal("you are not logged in")
		}
		project := project.NewProject()
		if err := project.Load(dir); err != nil {
			logger.Fatal("failed to load project: %s", err)
		}
		projectData, err := project.SetProjectEnv(logger, apiUrl, apiKey, map[string]interface{}{args[0]: args[1]})
		if err != nil {
			logger.Fatal("failed to set project env: %s", err)
		}
		for key, value := range projectData.Env {
			if key == args[0] {
				fmt.Printf("%s=%s\n", key, value)
			}
		}
	},
}


var envGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get environment variables",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		dir := resolveProjectDir(logger, cmd)
		apiUrl := viper.GetString("overrides.api_url")
		apiKey := viper.GetString("auth.api_key")
		if apiKey == "" {
			logger.Fatal("you are not logged in")
		}
		project := project.NewProject()
		if err := project.Load(dir); err != nil {
			logger.Fatal("failed to load project: %s", err)
		}
		projectData, err := project.ListProjectEnv(logger, apiUrl, apiKey)
		if err != nil {
			logger.Fatal("failed to list project env: %s", err)
		}
		for key, value := range projectData.Env {
			if key == args[0] {
				fmt.Printf("%s=%s\n", key, value)
			}
		}
	},
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all environment variables",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		dir := resolveProjectDir(logger, cmd)
		apiUrl := viper.GetString("overrides.api_url")
		apiKey := viper.GetString("auth.api_key")
		if apiKey == "" {
			logger.Fatal("you are not logged in")
		}
		project := project.NewProject()
		if err := project.Load(dir); err != nil {
			logger.Fatal("failed to load project: %s", err)
		}
		projectData, err := project.ListProjectEnv(logger, apiUrl, apiKey)
		if err != nil {
			logger.Fatal("failed to list project env: %s", err)
		}
		for key, value := range projectData.Env {
			fmt.Printf("%s=%s\n", key, value)
		}
	},
}


func init() {
	rootCmd.AddCommand(envCmd)
	envCmd.AddCommand(envSetCmd)
	envCmd.AddCommand(envListCmd )
	envCmd.AddCommand(envGetCmd)
}
