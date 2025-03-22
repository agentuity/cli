package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/agentuity/cli/internal/mcp"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/env"
	mcp_golang "github.com/agentuity/mcp-golang/v2"
	"github.com/agentuity/mcp-golang/v2/transport"
	"github.com/agentuity/mcp-golang/v2/transport/stdio"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP commands",
	Long: `Manage MCP commands.

Flags:
  --long    Print the long version including commit hash and build date

Examples:
  agentuity mcp`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the Agentuity CLI as an MCP server",
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
	Use:   "uninstall",
	Short: "Uninstall the Agentuity CLI as an MCP server",
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

var mcpRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Agentuity MCP server",
	Long: `Run the Agentuity MCP server.

Examples:
  agentuity mcp run
  agentuity mcp run --cli
	agentuity mcp run --sse`,
	Run: func(cmd *cobra.Command, args []string) {
		cli, _ := cmd.Flags().GetBool("cli")
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		var t transport.Transport
		if cli {
			t = stdio.NewStdioServerTransport()
		} else {
			logger.Fatal("SSE mode is not yet implemented")
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
		}
		err := mcp.Register(mcpContext)
		if err != nil {
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

	mcpRunCmd.Flags().Bool("cli", true, "Run the MCP server in CLI mode")
	mcpRunCmd.Flags().Bool("sse", false, "Run the MCP server in SSE mode")
	mcpRunCmd.MarkFlagsMutuallyExclusive("cli", "sse")
}
