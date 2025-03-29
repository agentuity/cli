package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/mcp"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/tui"
	mcp_golang "github.com/agentuity/mcp-golang/v2"
	"github.com/agentuity/mcp-golang/v2/transport"
	"github.com/agentuity/mcp-golang/v2/transport/stdio"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Args:  cobra.NoArgs,
	Short: "Manage MCP commands",
	Long: `Manage MCP commands.

The Agentuity CLI implements the Model Context Protocol (MCP).  The Agentuity
CLI can be configured with a MCP client (such as Cursor, Windsurf, Claude Desktop etc)
to increase the capabilities of the AI Agent inside the client.

For more information on the MCP protocol, see https://modelcontextprotocol.io/

Examples:
  agentuity mcp install
  agentuity mcp uninstall
  agentuity mcp list`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var mcpInstallCmd = &cobra.Command{
	Use:     "install",
	Args:    cobra.NoArgs,
	Aliases: []string{"i", "add"},
	Short:   "Install the Agentuity CLI as an MCP server",
	Long: `Install the Agentuity CLI as an MCP server.

Examples:
  agentuity mcp install`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		if err := mcp.Install(ctx, logger); err != nil {
			logger.Fatal("%s", err)
		}
	},
}

var mcpUninstallCmd = &cobra.Command{
	Use:     "uninstall",
	Args:    cobra.NoArgs,
	Aliases: []string{"rm", "delete", "del", "remove"},
	Short:   "Uninstall the Agentuity CLI as an MCP server",
	Long: `Uninstall the Agentuity CLI as an MCP server.

Examples:
  agentuity mcp uninstall`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		if err := mcp.Uninstall(ctx, logger); err != nil {
			logger.Fatal("%s", err)
		}
	},
}

var mcpListCmd = &cobra.Command{
	Use:     "list",
	Args:    cobra.NoArgs,
	Aliases: []string{"ls"},
	Short:   "List the MCP server configurations",
	Long: `List the MCP server configurations.

Examples:
  agentuity mcp list`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		detected, err := mcp.Detect(true)
		if err != nil {
			logger.Fatal("%s", err)
		}
		if len(detected) == 0 {
			tui.ShowWarning("No MCP servers detected on this machine")
			return
		}
		var needsInstall int
		for _, config := range detected {
			if config.Installed && config.Detected {
				tui.ShowSuccess("%s %s", tui.Bold(tui.PadRight(config.Name, 20, " ")), tui.Muted("configured"))
			} else if config.Installed {
				tui.ShowError("%s %s", tui.Bold(tui.PadRight(config.Name, 20, " ")), tui.Muted("not configured"))
				needsInstall++
			} else {
				tui.ShowWarning("%s %s", tui.Bold(tui.PadRight(config.Name, 20, " ")), tui.Muted("not installed"))
			}
		}
		if needsInstall > 0 {
			fmt.Println()
			text := "client"
			if needsInstall > 1 {
				text = "clients"
			}
			tui.WaitForAnyKeyMessage(fmt.Sprintf("Press any key to install the Agentuity MCP server for the missing %s...", text))
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			if err := mcp.Install(ctx, logger); err != nil {
				logger.Fatal("%s", err)
			}
		}
	},
}

var mcpRunCmd = &cobra.Command{
	Use:    "run",
	Hidden: true,
	Args:   cobra.NoArgs,
	Short:  "Run the Agentuity MCP server",
	Long: `Run the Agentuity MCP server.

Examples:
  agentuity mcp run
  agentuity mcp run --stdio
	agentuity mcp run --sse`,
	Run: func(cmd *cobra.Command, args []string) {
		stdioTransport, _ := cmd.Flags().GetBool("stdio")
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		tmplDir, err := getConfigTemplateDir(cmd)
		if err != nil {
			errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from directory")).ShowErrorAndExit()
		}
		var t transport.Transport
		if stdioTransport {
			t = stdio.NewStdioServerTransport()
		} else {
			logger.Fatal("SSE mode is not yet supported")
		}
		project := project.TryProject(cmd)
		server := mcp_golang.NewServer(t)
		mcpContext := mcp.MCPContext{
			Context:      ctx,
			Logger:       logger,
			Server:       server,
			Command:      cmd,
			ProjectDir:   project.Dir,
			APIKey:       project.Token,
			APIURL:       project.APIURL,
			AppURL:       project.APPURL,
			TransportURL: project.TransportURL,
			Project:      project.Project,
			LoggedIn:     project.Token != "",
			TemplateDir:  tmplDir,
		}
		if err := mcp.Register(mcpContext); err != nil {
			logger.Fatal("%s", err)
		}
		if err := server.Serve(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Info("bye")
				return
			}
			logger.Fatal("%s", err)
		}
		<-ctx.Done()
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpInstallCmd)
	mcpCmd.AddCommand(mcpUninstallCmd)
	mcpCmd.AddCommand(mcpRunCmd)
	mcpCmd.AddCommand(mcpListCmd)

	mcpRunCmd.Flags().Bool("stdio", true, "Run the MCP server in Stdio mode")
	mcpRunCmd.Flags().Bool("sse", false, "Run the MCP server in SSE mode")
	mcpRunCmd.MarkFlagsMutuallyExclusive("stdio", "sse")
}
