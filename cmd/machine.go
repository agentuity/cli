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

	// Flags for machine list command
	machineListCmd.Flags().String("format", "table", "Output format (table, json)")

	// Flags for machine remove command
	machineRemoveCmd.Flags().Bool("force", false, "Force removal without confirmation")

	// Flags for machine status command
	machineStatusCmd.Flags().String("format", "table", "Output format (table, json)")
}
