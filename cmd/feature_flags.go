package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Feature flag constants
const (
	FeaturePromptsEvals = "enable_prompts_evals"
)

// SetupFeatureFlags initializes feature flag configuration and command line flags
func SetupFeatureFlags(rootCmd *cobra.Command) {
	// Set default values
	viper.SetDefault("features.enable_prompts_evals", false)
	// Add command line flags
	rootCmd.PersistentFlags().Bool("enable-prompts-evals", false, "Enable prompts evals")
	rootCmd.PersistentFlags().MarkHidden("enable-prompts-evals")
	viper.BindPFlag("features.enable_prompts_evals", rootCmd.PersistentFlags().Lookup("enable-prompts-evals"))
}

// Feature flag helper functions

// IsFeatureEnabled checks if a feature flag is enabled
func IsFeatureEnabled(feature string) bool {
	return viper.GetBool("features." + feature)
}

// IsExperimentalEnabled checks if experimental features are enabled
func IsPromptsEvalsEnabled() bool {
	return IsFeatureEnabled(FeaturePromptsEvals)
}

// GetEnabledFeatures returns a list of all currently enabled feature flags
func GetEnabledFeatures() []string {
	var enabled []string

	if IsPromptsEvalsEnabled() {
		enabled = append(enabled, FeaturePromptsEvals)
	}
	return enabled
}

// IsAnyFeatureEnabled checks if any feature flags are enabled
func IsAnyFeatureEnabled() bool {
	return IsPromptsEvalsEnabled()
}
