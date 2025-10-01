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
	// Add command line flags (no defaults set - only write to config when explicitly enabled)
	rootCmd.PersistentFlags().Bool("enable-prompts-evals", false, "Enable prompts evals")
	rootCmd.PersistentFlags().MarkHidden("enable-prompts-evals")
	// Don't bind to viper - we'll handle this manually to prevent false values in config
}

// Feature flag helper functions

// IsFeatureEnabled checks if a feature flag is enabled
func IsFeatureEnabled(feature string) bool {
	key := "features." + feature
	if viper.IsSet(key) {
		return viper.GetBool(key)
	}
	return false
}

// CheckFeatureFlag checks if a feature flag is enabled via command line or config
// and only writes to config if explicitly enabled
func CheckFeatureFlag(cmd *cobra.Command, feature string, cmdFlagName string) bool {
	// Check if flag was set via command line
	if cmd.Flags().Changed(cmdFlagName) {
		enabled, _ := cmd.Flags().GetBool(cmdFlagName)
		// Only write to config if explicitly enabled
		if enabled {
			viper.Set("features."+feature, true)
		}
		return enabled
	}

	// Check config file
	return IsFeatureEnabled(feature)
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
