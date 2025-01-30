package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/inancgumus/screen"
	"github.com/shopmonkeyus/go-common/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

const logoHeader = `
                                       ###                                      
                                      #####                                     
                                    ########.                                   
                                   ###########                                  
                                 (#####   #####                                 
                                #####/     ######                               
                              *#####         #####                              
                             ######           #####                            
                           .#################################                   
                          ####################################                  
                                                                                
                                                                                
              #############################################                     
             ###############################################                    
                   ######                               ######                  
                 ,#####                                  *#####                 
                ######                                     #####               
              .###################################################              
             #######################################################            
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
	fmt.Println()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "agentuity",
	Aliases: []string{"ag"},
	Short:   color.RGB(0, 255, 255).Sprint(center("Agentuity Cloud Platform Tooling", 81)),
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
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/agentuity/config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "The log level to use")
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

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		return
	}

	viper.SetDefault("overrides.app_url", "https://app.agentuity.com")
}

func flagOrEnv(cmd *cobra.Command, flagName string, envName string, defaultValue string) string {
	flagValue, _ := cmd.Flags().GetString(flagName)
	if flagValue != "" {
		return flagValue
	}
	if val, ok := os.LookupEnv(envName); ok {
		return val
	}
	return defaultValue
}

func newLogger(cmd *cobra.Command) logger.Logger {
	log.SetFlags(0)
	level := flagOrEnv(cmd, "log-level", "AGENTUITY_LOG_LEVEL", "info")
	switch level {
	case "debug", "DEBUG":
		return logger.NewConsoleLogger(logger.LevelDebug)
	case "warn", "WARN":
		return logger.NewConsoleLogger(logger.LevelWarn)
	case "error", "ERROR":
		return logger.NewConsoleLogger(logger.LevelError)
	case "trace", "TRACE":
		return logger.NewConsoleLogger(logger.LevelTrace)
	case "info", "INFO":
	default:
	}
	return logger.NewConsoleLogger(logger.LevelInfo)
}

func printSuccess(msg string) {
	color.Green("✓ %s", msg)
}

func initScreenWithLogo() {
	screen.Clear()
	screen.MoveTopLeft()
	printLogo()
	fmt.Println()
	fmt.Println()

}
