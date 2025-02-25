package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/fatih/color"
	"github.com/inancgumus/screen"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Version string

var cfgFile string

const logoHeader = `
                 ---                 
                -----                
               -------               
              ---- ----              
             ----   ----             
            ----     ----            
           ----       ----           
          ----------------------     
         ------------------------    


 -------------------------------     
---------------------------------    
   ----                       ----   
  ----                         ----  
 ----------------------------------- 
-------------------------------------
`

func center(s string, width int) string {
	padding := width - len(s)
	if padding <= 0 {
		return s
	}
	leftPadding := padding / 2
	rightPadding := padding - leftPadding
	return strings.Repeat(" ", leftPadding) + s + strings.Repeat(" ", rightPadding)
}

func printLogo() {
	color.RGB(0, 255, 255).Print(logoHeader)
	fmt.Println(color.RGB(0, 255, 255).Sprint(center("Agentuity Agent Cloud", 40)))
	fmt.Println()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "agentuity",
	Aliases: []string{"ag"},
	Run: func(cmd *cobra.Command, args []string) {
		printLogo()
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/agentuity/config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "The log level to use")

	rootCmd.PersistentFlags().String("app-url", "https://app.agentuity.com", "The base url of the Agentuity Console app")
	rootCmd.PersistentFlags().MarkHidden("app-url")
	viper.BindPFlag("overrides.app_url", rootCmd.PersistentFlags().Lookup("app-url"))

	rootCmd.PersistentFlags().String("api-url", "https://api.agentuity.com", "The base url of the Agentuity API")
	rootCmd.PersistentFlags().MarkHidden("api-url")
	viper.BindPFlag("overrides.api_url", rootCmd.PersistentFlags().Lookup("api-url"))

	rootCmd.PersistentFlags().String("websocket-url", "wss://api.agentuity.com", "The base url of the Agentuity WebSocket API")
	rootCmd.PersistentFlags().MarkHidden("websocket-url")
	viper.BindPFlag("overrides.websocket_url", rootCmd.PersistentFlags().Lookup("websocket-url"))

	viper.SetDefault("overrides.app_url", "https://app.agentuity.com")
	viper.SetDefault("overrides.api_url", "https://api.agentuity.com")

	cobra.OnInitialize(initConfig)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		dir := filepath.Join(home, ".config", "agentuity")
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0700); err != nil {
				log.Fatalf("failed to create config directory (%s): %s", dir, err)
			}
		}
		cfgFile = filepath.Join(dir, "config.yaml")
		viper.SetConfigFile(cfgFile)
	}

	viper.AutomaticEnv() // read in environment variables that match

	// Finally read the config file
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %s\n", err)
	}
}

func printSuccess(msg string, args ...any) {
	fmt.Printf("%s %s", color.GreenString("✓"), color.WhiteString(fmt.Sprintf(msg, args...)))
	fmt.Println()
}

func printLock(msg string, args ...any) {
	fmt.Printf("🔒 %s", fmt.Sprintf(msg, args...))
	fmt.Println()
}

func printWarning(msg string, args ...any) {
	fmt.Printf("%s %s", color.RedString("✕"), fmt.Sprintf(msg, args...))
	fmt.Println()
}

func command(cmd string, args ...string) string {
	cmdline := "agentuity " + strings.Join(append([]string{cmd}, args...), " ")
	return color.HiCyanString(cmdline)
}

func link(url string, args ...any) string {
	val := color.HiWhiteString(fmt.Sprintf(url, args...))
	if runtime.GOOS == "windows" {
		return val
	}
	return "\033[4m" + val + "\033[0m"
}

func maxString(val string, max int) string {
	if len(val) > max {
		return val[:max] + "..."
	}
	return val
}

func showSpinner(logger logger.Logger, title string, action func()) {
	if err := spinner.New().Title(title).Action(action).Run(); err != nil {
		logger.Fatal("%s", err)
	}
}

var theme = huh.ThemeCatppuccin()

func getInput(logger logger.Logger, title string, description string, prompt string, mask bool, defaultValue string, validate func(string) error) string {
	value := defaultValue
	if prompt == "" {
		prompt = "> "
	}
	echoMode := huh.EchoModeNormal
	if mask {
		echoMode = huh.EchoModePassword
	}
	if validate == nil {
		validate = func(string) error {
			return nil
		}
	}
	if huh.NewInput().
		Title(title).
		Description(description).
		Prompt(prompt).
		Value(&value).
		EchoMode(echoMode).
		Validate(validate).
		WithHeight(100).WithTheme(theme).Run() != nil {
		logger.Fatal("failed to get input value")
	}
	return value
}

func ask(logger logger.Logger, title string, defaultValue bool) bool {
	confirm := defaultValue
	if huh.NewConfirm().
		Title(title).
		Affirmative("Yes!").
		Negative("No").
		Value(&confirm).
		Inline(true).
		WithTheme(theme).
		Run() != nil {
		logger.Fatal("failed to confirm")
	}
	return confirm
}

func initScreenWithLogo() {
	screen.Clear()
	screen.MoveTopLeft()
	printLogo()
	fmt.Println()
	fmt.Println()
}

func createPromptHelper() provider.PromptHelpers {
	return provider.PromptHelpers{
		ShowSpinner:   showSpinner,
		PrintSuccess:  printSuccess,
		CommandString: command,
		LinkString:    link,
		PrintLock:     printLock,
		PrintWarning:  printWarning,
		Ask:           ask,
		PromptForEnv:  promptForEnv,
	}
}

func resolveDir(logger logger.Logger, dir string, createIfNotExists bool) string {
	if dir == "." {
		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatal("failed to get current directory: %s", err)
		}
		dir = cwd
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if createIfNotExists {
			if err := os.MkdirAll(dir, 0700); err != nil {
				logger.Fatal("failed to create directory: %s", err)
			}
		} else {
			logger.Fatal("directory does not exist: %s", dir)
		}
	}
	return dir
}

func resolveProjectDir(logger logger.Logger, cmd *cobra.Command) string {
	cwd, err := os.Getwd()
	if err != nil {
		logger.Fatal("failed to get current directory: %s", err)
	}
	dir := cwd
	dirFlag, _ := cmd.Flags().GetString("dir")
	if dirFlag != "" {
		dir = dirFlag
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		logger.Fatal("failed to get absolute path: %s", err)
	}
	if !project.ProjectExists(abs) {
		logger.Fatal("no agentuity.yaml file found in the current directory")
	}
	return abs
}

func getURLs(logger logger.Logger) (string, string) {
	appUrl := viper.GetString("overrides.app_url")
	apiUrl := viper.GetString("overrides.api_url")
	if apiUrl == "https://api.agentuity.com" && appUrl != "https://app.agentuity.com" {
		logger.Debug("switching app url to production since the api url is production")
		appUrl = "https://app.agentuity.com"
	} else if apiUrl == "https://api.agentuity.div" && appUrl == "https://app.agentuity.com" {
		logger.Debug("switching app url to dev since the api url is dev")
		appUrl = "http://localhost:3000"
	}
	return apiUrl, appUrl
}
