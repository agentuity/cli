package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/sys"
	"github.com/spf13/cobra"
)

type profile struct {
	name     string
	filename string
	selected bool
}

var profileNameRegex = regexp.MustCompile(`name:\s+["]?([\w-_]+)["]?`)
var profileNameValidRegex = regexp.MustCompile(`^[\w-_]{3,}$`)

func saveProfile(name string) {
	fn := filepath.Join(filepath.Dir(cfgFile), "profile")
	os.WriteFile(fn, []byte(name), 0644)
}

func getProfile() string {
	dir := filepath.Dir(cfgFile)
	fn := filepath.Join(dir, "profile")
	if util.Exists(fn) {
		buf, _ := os.ReadFile(fn)
		filename := strings.TrimSpace(string(buf))
		if util.Exists(filename) {
			return filename
		}
	}
	return cfgFile
}

func fetchProfiles() []profile {
	dir := filepath.Dir(cfgFile)
	var profiles []profile
	files, _ := sys.ListDir(dir)
	for _, file := range files {
		if filepath.Ext(file) == ".yaml" {
			buf, _ := os.ReadFile(file)
			m := profileNameRegex.FindStringSubmatch(string(buf))
			if len(m) > 0 {
				profiles = append(profiles, profile{m[1], file, file == cfgFile})
			}
		}
	}
	return profiles
}

func selectProfile(logger logger.Logger) string {
	profiles := fetchProfiles()
	var opts []tui.Option
	for _, p := range profiles {
		prepend := " "
		if p.selected {
			prepend = "•"
		}
		opts = append(opts, tui.Option{Selected: p.selected, ID: p.filename, Text: prepend + " " + tui.PadRight(p.name, 15, " ") + " " + tui.Muted(filepath.Base(filepath.Dir(p.filename))+"/"+filepath.Base(p.filename))})
	}
	return tui.Select(logger, "Select a profile\n", "", opts)
}

var profileCmd = &cobra.Command{
	Use:    "profile",
	Args:   cobra.NoArgs,
	Short:  "Manage profiles",
	Long: `Manage CLI configuration profiles.

Use the subcommands to create and switch between different configuration profiles.`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var profileUseCmd = &cobra.Command{
	Use:    "use [name]",
	Args:   cobra.MaximumNArgs(1),
	Short:  "Use a different profile",
	Long: `Switch to a different configuration profile.

Arguments:
  [name]    The name of the profile to use

If no name is provided, you will be prompted to select a profile.

Examples:
  agentuity profile use dev
  agentuity profile use`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		var name string
		if len(args) > 0 {
			name = args[0]
			profiles := fetchProfiles()
			var found bool
			for _, profile := range profiles {
				if profile.name == name {
					name = profile.filename
					found = true
					break
				}
			}
			if !found {
				name = ""
			}
		}
		if name == "" {
			name = selectProfile(logger)
		}
		saveProfile(name)
	},
}

var profileCreateCmd = &cobra.Command{
	Use:    "create",
	Args:   cobra.NoArgs,
	Short:  "Create a new empty profile",
	Long: `Create a new empty configuration profile.

This command creates a new configuration profile with a unique name.
You will be prompted to enter a name for the profile.

Examples:
  agentuity profile create`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		profiles := fetchProfiles()
		name := tui.InputWithValidation(logger, "Name your profile", "Choose a short unique name", 0, func(val string) error {
			if val == "" {
				return fmt.Errorf("profile name cannot be empty")
			}
			if !profileNameValidRegex.MatchString(val) {
				return fmt.Errorf("profile name must match: %s", profileNameValidRegex.String())
			}
			for _, profile := range profiles {
				if profile.name == val {
					return fmt.Errorf("profile name %s already exists", val)
				}
			}
			return nil
		})
		fp := filepath.Join(filepath.Dir(cfgFile), util.SafeFilename(name)+".yaml")
		os.WriteFile(fp, []byte(fmt.Sprintf(`name: "%s"`, name)+"\n"), 0644)
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileCreateCmd)
}
