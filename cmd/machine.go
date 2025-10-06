package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/infrastructure"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
)

var machineCmd = &cobra.Command{
	Use:   "machine",
	Short: "Machine management commands",
	Long: `Machine management commands for listing and managing infrastructure machines.

Use the subcommands to manage your machines.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var machineListCmd = &cobra.Command{
	Use:     "list [cluster]",
	GroupID: "info",
	Short:   "List all machines",
	Long: `List all infrastructure machines, optionally filtered by cluster.

Arguments:
  [cluster]    The cluster name or ID to filter machines (optional)

Examples:
  agentuity machine list
  agentuity machine ls production
  agentuity machine list cluster-001 --format json`,
	Aliases: []string{"ls"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// Check if clustering is enabled for machine operations
		infrastructure.EnsureMachineClusteringEnabled(ctx, logger, apiUrl, apikey)

		var clusterFilter string
		if len(args) > 0 {
			clusterFilter = args[0]
		}

		format, _ := cmd.Flags().GetString("format")
		if format != "" {
			if err := validateFormat(format); err != nil {
				errsystem.New(errsystem.ErrInvalidArgumentProvided, err, errsystem.WithContextMessage("Invalid output format")).ShowErrorAndExit()
			}
		}

		var machines []infrastructure.Machine

		tui.ShowSpinner("Fetching machines...", func() {
			var err error
			machines, err = infrastructure.ListMachines(ctx, logger, apiUrl, apikey, clusterFilter)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list machines")).ShowErrorAndExit()
			}
		})

		if format == "json" {
			outputJSON(machines)
			return
		}

		if len(machines) == 0 {
			fmt.Println()
			if clusterFilter != "" {
				tui.ShowWarning("no machines found in cluster %s", clusterFilter)
			} else {
				tui.ShowWarning("no machines found")
			}
			return
		}

		// Sort machines by cluster, then by instance ID
		sort.Slice(machines, func(i, j int) bool {
			if machines[i].ClusterID != machines[j].ClusterID {
				return machines[i].ClusterID < machines[j].ClusterID
			}
			return machines[i].InstanceID < machines[j].InstanceID
		})

		// Always use table format since we don't have cluster names for grouping
		headers := []string{
			tui.Title("ID"),
			tui.Title("Instance ID"),
			tui.Title("Cluster ID"),
			tui.Title("Status"),
			tui.Title("Provider"),
			tui.Title("Region"),
			tui.Title("Started"),
		}

		rows := [][]string{}
		for _, machine := range machines {
			var statusColor string
			switch machine.Status {
			case "running":
				statusColor = tui.Bold(machine.Status)
			case "provisioned":
				statusColor = tui.Text(machine.Status)
			case "stopping", "stopped", "paused":
				statusColor = tui.Warning(machine.Status)
			case "error":
				statusColor = tui.Warning(machine.Status)
			default:
				statusColor = tui.Text(machine.Status)
			}

			// Format started time or use created time
			startedTime := ""
			if machine.StartedAt != nil {
				startedTime = (*machine.StartedAt)[:10] // show date only
			} else {
				startedTime = machine.CreatedAt[:10]
			}

			rows = append(rows, []string{
				tui.Muted(machine.ID),
				tui.Text(machine.InstanceID),
				tui.Muted(machine.ClusterID),
				statusColor,
				tui.Text(machine.Provider),
				tui.Text(machine.Region),
				tui.Muted(startedTime),
			})
		}

		tui.Table(headers, rows)

	},
}

var machineRemoveCmd = &cobra.Command{
	Use:     "remove [id]",
	GroupID: "management",
	Short:   "Remove a machine",
	Long: `Remove an infrastructure machine by ID.

This command will terminate the specified machine and remove it from the cluster.

Arguments:
  [id]    The ID of the machine to remove

Examples:
  agentuity machine remove machine-001
  agentuity machine rm machine-001 --force`,
	Aliases: []string{"rm", "del"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// Check if clustering is enabled for machine operations
		infrastructure.EnsureMachineClusteringEnabled(ctx, logger, apiUrl, apikey)

		machineID := args[0]
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			if !tui.Ask(logger, fmt.Sprintf("Are you sure you want to remove machine %s? This action cannot be undone.", machineID), false) {
				tui.ShowWarning("cancelled")
				return
			}
		}

		tui.ShowSpinner(fmt.Sprintf("Removing machine %s...", machineID), func() {
			if err := infrastructure.DeleteMachine(ctx, logger, apiUrl, apikey, machineID); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to remove machine")).ShowErrorAndExit()
			}
		})

		tui.ShowSuccess("Machine %s removed successfully", machineID)

	},
}

var machineStatusCmd = &cobra.Command{
	Use:     "status [id]",
	GroupID: "info",
	Short:   "Get machine status",
	Long: `Get the detailed status of a specific machine.

Arguments:
  [id]    The ID of the machine

Examples:
  agentuity machine status machine-001
  agentuity machine status machine-001 --format json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// Check if clustering is enabled for machine operations
		infrastructure.EnsureMachineClusteringEnabled(ctx, logger, apiUrl, apikey)

		machineID := args[0]
		format, _ := cmd.Flags().GetString("format")

		if format != "" {
			if err := validateFormat(format); err != nil {
				errsystem.New(errsystem.ErrInvalidArgumentProvided, err, errsystem.WithContextMessage("Invalid output format")).ShowErrorAndExit()
			}
		}

		var machine *infrastructure.Machine

		tui.ShowSpinner(fmt.Sprintf("Fetching machine %s status...", machineID), func() {
			var err error
			machine, err = infrastructure.GetMachine(ctx, logger, apiUrl, apikey, machineID)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get machine status")).ShowErrorAndExit()
			}
		})

		if format == "json" {
			outputJSON(machine)
			return
		}

		fmt.Printf("Machine ID: %s\n", tui.Bold(machine.ID))
		fmt.Printf("Instance ID: %s\n", machine.InstanceID)
		fmt.Printf("Cluster ID: %s\n", machine.ClusterID)
		if machine.ClusterName != nil {
			fmt.Printf("Cluster Name: %s\n", *machine.ClusterName)
		}
		fmt.Printf("Status: %s\n", machine.Status)
		fmt.Printf("Provider: %s\n", machine.Provider)
		fmt.Printf("Region: %s\n", machine.Region)
		if machine.OrgID != nil {
			fmt.Printf("Organization ID: %s\n", *machine.OrgID)
		}
		if machine.OrgName != nil {
			fmt.Printf("Organization: %s\n", *machine.OrgName)
		}
		fmt.Printf("Created: %s\n", machine.CreatedAt)
		if machine.UpdatedAt != nil {
			fmt.Printf("Updated: %s\n", *machine.UpdatedAt)
		}

		if machine.StartedAt != nil {
			fmt.Printf("Started: %s\n", *machine.StartedAt)
		}
		if machine.StoppedAt != nil {
			fmt.Printf("Stopped: %s\n", *machine.StoppedAt)
		}
		if machine.PausedAt != nil {
			fmt.Printf("Paused: %s\n", *machine.PausedAt)
		}
		if machine.ErroredAt != nil {
			fmt.Printf("Errored: %s\n", *machine.ErroredAt)
		}
		if machine.Error != nil {
			fmt.Printf("Error: %s\n", *machine.Error)
		}

		// Display metadata if present
		if len(machine.Metadata) > 0 {
			fmt.Printf("Metadata:\n")
			for key, value := range machine.Metadata {
				fmt.Printf("  %s: %v\n", key, value)
			}
		}

	},
}

var machineCreateCmd = &cobra.Command{
	Use:     "create [cluster_id] [provider] [region]",
	GroupID: "info",
	Short:   "Create a new machine for a cluster",
	Long: `Create a new machine for a cluster.

Arguments:
  [cluster_id]  The cluster ID to create a machine in (optional in interactive mode)
  [provider]    The cloud provider (optional in interactive mode)  
  [region]      The region to deploy in (optional in interactive mode)

Examples:
  agentuity machine create
  agentuity machine create cluster-001 aws us-east-1`,
	Args:    cobra.MaximumNArgs(3),
	Aliases: []string{"new"},
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// Check if clustering is enabled for machine operations
		infrastructure.EnsureMachineClusteringEnabled(ctx, logger, apiUrl, apikey)

		var clusterID, provider, region string

		// If all arguments provided, use them directly
		if len(args) == 3 {
			clusterID = args[0]
			provider = args[1]
			region = args[2]
		} else if tui.HasTTY {
			// Interactive mode - prompt for missing values
			cluster := promptForClusterSelection(ctx, logger, apiUrl, apikey)
			provider = cluster.Provider
			region = promptForRegionSelection(ctx, logger, provider)
			clusterID = cluster.ID
		} else {
			// Non-interactive mode - require all arguments
			errsystem.New(errsystem.ErrMissingRequiredArgument, fmt.Errorf("cluster_id, provider, and region are required in non-interactive mode"), errsystem.WithContextMessage("Missing required arguments")).ShowErrorAndExit()
		}

		orgId := promptForClusterOrganization(ctx, logger, cmd, apiUrl, apikey, "What organization should we create the machine in?")

		resp, err := infrastructure.CreateMachine(ctx, logger, apiUrl, apikey, clusterID, orgId, provider, region)
		if err != nil {
			logger.Fatal("error creating machine: %s", err)
		}
		fmt.Printf("Machine created successfully with ID: %s and Token: %s\n", resp.ID, resp.Token)
	},
}

func init() {
	// Add command groups for machine operations
	machineCmd.AddGroup(&cobra.Group{
		ID:    "management",
		Title: "Machine Management:",
	})
	machineCmd.AddGroup(&cobra.Group{
		ID:    "info",
		Title: "Information:",
	})

	rootCmd.AddCommand(machineCmd)
	machineCmd.AddCommand(machineListCmd)
	machineCmd.AddCommand(machineRemoveCmd)
	machineCmd.AddCommand(machineStatusCmd)
	machineCmd.AddCommand(machineCreateCmd)

	// Flags for machine list command
	machineListCmd.Flags().String("format", "table", "Output format (table, json)")

	// Flags for machine remove command
	machineRemoveCmd.Flags().Bool("force", false, "Force removal without confirmation")

	// Flags for machine status command
	machineStatusCmd.Flags().String("format", "table", "Output format (table, json)")

}

// promptForClusterSelection prompts the user to select a cluster from available clusters
func promptForClusterSelection(ctx context.Context, logger logger.Logger, apiUrl, apikey string) infrastructure.Cluster {
	clusters, err := infrastructure.ListClusters(ctx, logger, apiUrl, apikey)
	if err != nil {
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list clusters")).ShowErrorAndExit()
	}

	if len(clusters) == 0 {
		errsystem.New(errsystem.ErrApiRequest, fmt.Errorf("no clusters found"), errsystem.WithUserMessage("No clusters found. Please create a cluster first using 'agentuity cluster create'")).ShowErrorAndExit()
	}

	if len(clusters) == 1 {
		cluster := clusters[0]
		fmt.Printf("Using cluster: %s (%s)\n", cluster.Name, cluster.ID)
		return cluster
	}

	// Sort clusters by Name then ID for deterministic display order
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Name != clusters[j].Name {
			return clusters[i].Name < clusters[j].Name
		}
		return clusters[i].ID < clusters[j].ID
	})

	var opts []tui.Option
	for _, cluster := range clusters {
		displayText := fmt.Sprintf("%s (%s) - %s %s", cluster.Name, cluster.ID, cluster.Provider, cluster.Region)
		opts = append(opts, tui.Option{ID: cluster.ID, Text: displayText})
	}

	id := tui.Select(logger, "Select a cluster to create a machine in:", "Choose the cluster where you want to deploy the new machine", opts)

	// Handle user cancellation (empty string)
	if id == "" {
		errsystem.New(errsystem.ErrApiRequest, fmt.Errorf("no cluster selected"), errsystem.WithUserMessage("No cluster selected")).ShowErrorAndExit()
	}

	// Find the selected cluster
	for _, cluster := range clusters {
		if cluster.ID == id {
			return cluster
		}
	}

	// This should never happen, but handle it as an impossible path
	errsystem.New(errsystem.ErrApiRequest, fmt.Errorf("selected cluster not found: %s", id), errsystem.WithUserMessage("Selected cluster not found")).ShowErrorAndExit()
	return infrastructure.Cluster{} // This line will never be reached
}

// promptForRegionSelection prompts the user to select a region
func promptForRegionSelection(ctx context.Context, logger logger.Logger, provider string) string {
	// Get regions for the provider (reuse the same logic from cluster.go)
	fmt.Println("Provider:", provider)
	opts := getRegionsForProvider(provider)
	return tui.Select(logger, "Which region should we use?", "The region to deploy the machine", opts)
}
