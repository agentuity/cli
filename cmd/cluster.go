package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/infrastructure"
	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Provider types for infrastructure
var validProviders = []string{"gcp", "aws", "azure", "vmware", "other"}

// Size types for clusters
var validSizes = []string{"dev", "small", "medium", "large"}

// Output formats
var validFormats = []string{"table", "json"}

func validateProvider(provider string) error {
	for _, p := range validProviders {
		if p == provider {
			return nil
		}
	}
	return fmt.Errorf("invalid provider %s, must be one of: %s", provider, validProviders)
}

func validateSize(size string) error {
	for _, s := range validSizes {
		if s == size {
			return nil
		}
	}
	return fmt.Errorf("invalid size %s, must be one of: %s", size, validSizes)
}

func validateFormat(format string) error {
	for _, f := range validFormats {
		if f == format {
			return nil
		}
	}
	return fmt.Errorf("invalid format %s, must be one of: %s", format, validFormats)
}

func outputJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

func promptForClusterOrganization(ctx context.Context, logger logger.Logger, cmd *cobra.Command, apiUrl string, token string) string {
	orgs, err := organization.ListOrganizations(ctx, logger, apiUrl, token)
	if err != nil {
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list organizations")).ShowErrorAndExit()
	}
	if len(orgs) == 0 {
		logger.Fatal("you are not a member of any organizations")
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("You are not a member of any organizations")).ShowErrorAndExit()
	}
	var orgId string
	if len(orgs) == 1 {
		orgId = orgs[0].OrgId
	} else {
		hasCLIFlag := cmd.Flags().Changed("org-id")
		prefOrgId, _ := cmd.Flags().GetString("org-id")
		if prefOrgId == "" {
			prefOrgId = viper.GetString("preferences.orgId")
		}
		if tui.HasTTY && !hasCLIFlag {
			var opts []tui.Option
			for _, org := range orgs {
				opts = append(opts, tui.Option{ID: org.OrgId, Text: org.Name, Selected: prefOrgId == org.OrgId})
			}
			orgId = tui.Select(logger, "What organization should we create the cluster in?", "", opts)
			viper.Set("preferences.orgId", orgId)
			viper.WriteConfig() // remember the preference
		} else {
			for _, org := range orgs {
				if org.OrgId == prefOrgId || org.Name == prefOrgId {
					return org.OrgId
				}
			}
			logger.Fatal("no TTY and no organization preference found. re-run with --org-id")
		}
	}
	return orgId
}

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Cluster management commands",
	Long: `Cluster management commands for creating, listing, and managing infrastructure clusters.

Use the subcommands to manage your clusters.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var clusterNewCmd = &cobra.Command{
	Use:     "new [name]",
	GroupID: "management",
	Short:   "Create a new cluster",
	Long: `Create a new infrastructure cluster with the specified configuration.

Arguments:
  [name]    The name of the cluster

Examples:
  agentuity cluster new production --provider gcp --size large --region us-west1
  agentuity cluster create staging --provider aws --size medium --region us-east-1`,
	Aliases: []string{"create"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		var name string
		if len(args) > 0 {
			name = args[0]
		}

		// Get organization ID
		orgId := promptForClusterOrganization(ctx, logger, cmd, apiUrl, apikey)

		provider, _ := cmd.Flags().GetString("provider")
		size, _ := cmd.Flags().GetString("size")
		region, _ := cmd.Flags().GetString("region")
		format, _ := cmd.Flags().GetString("format")

		// Validate inputs
		if provider != "" {
			if err := validateProvider(provider); err != nil {
				errsystem.New(errsystem.ErrInvalidArgumentProvided, err, errsystem.WithContextMessage("Invalid provider")).ShowErrorAndExit()
			}
		}

		if size != "" {
			if err := validateSize(size); err != nil {
				errsystem.New(errsystem.ErrInvalidArgumentProvided, err, errsystem.WithContextMessage("Invalid cluster size")).ShowErrorAndExit()
			}
		}

		if format != "" {
			if err := validateFormat(format); err != nil {
				errsystem.New(errsystem.ErrInvalidArgumentProvided, err, errsystem.WithContextMessage("Invalid output format")).ShowErrorAndExit()
			}
		}

		// Interactive prompts if TTY available and values not provided
		if tui.HasTTY {
			if name == "" {
				name = tui.Input(logger, "What should we name the cluster?", "A unique name for your cluster")
			}

			if provider == "" {
				opts := []tui.Option{}
				for _, p := range validProviders {
					opts = append(opts, tui.Option{ID: p, Text: p})
				}
				provider = tui.Select(logger, "Which provider should we use?", "", opts)
			}

			if size == "" {
				opts := []tui.Option{
					{ID: "dev", Text: "Development (small resources)"},
					{ID: "small", Text: "Small (basic production)"},
					{ID: "medium", Text: "Medium (standard production)"},
					{ID: "large", Text: "Large (high performance)"},
				}
				size = tui.Select(logger, "What size cluster do you need?", "", opts)
			}

			if region == "" {
				// TODO: move these to use an option based on the selected provider
				region = tui.Input(logger, "Which region should we use?", "The region to deploy the cluster")
			}
		} else {
			// Non-interactive validation
			if name == "" {
				errsystem.New(errsystem.ErrMissingRequiredArgument, fmt.Errorf("cluster name is required"), errsystem.WithContextMessage("Missing cluster name")).ShowErrorAndExit()
			}
			if provider == "" {
				errsystem.New(errsystem.ErrMissingRequiredArgument, fmt.Errorf("provider is required"), errsystem.WithContextMessage("Missing provider")).ShowErrorAndExit()
			}
			if size == "" {
				errsystem.New(errsystem.ErrMissingRequiredArgument, fmt.Errorf("size is required"), errsystem.WithContextMessage("Missing cluster size")).ShowErrorAndExit()
			}
			if region == "" {
				errsystem.New(errsystem.ErrMissingRequiredArgument, fmt.Errorf("region is required"), errsystem.WithContextMessage("Missing region")).ShowErrorAndExit()
			}
		}

		var cluster *infrastructure.Cluster

		tui.ShowSpinner("Creating cluster...", func() {
			var err error
			cluster, err = infrastructure.CreateCluster(ctx, logger, apiUrl, apikey, infrastructure.CreateClusterArgs{
				Name:     name,
				Provider: provider,
				Type:     size, // CLI uses "size" but backend expects "type"
				Region:   region,
				OrgID:    orgId,
			})
			if err != nil {
				errsystem.New(errsystem.ErrCreateProject, err, errsystem.WithContextMessage("Failed to create cluster")).ShowErrorAndExit()
			}
		})

		if format == "json" {
			outputJSON(cluster)
		} else {
			tui.ShowSuccess("Cluster %s created successfully with ID: %s", cluster.Name, cluster.ID)
			fmt.Printf("Provider: %s\n", cluster.Provider)
			fmt.Printf("Size: %s\n", cluster.Type) // backend field is "type" but display as "size"
			fmt.Printf("Region: %s\n", cluster.Region)
			fmt.Printf("Created: %s\n", cluster.CreatedAt)
		}

	},
}

var clusterListCmd = &cobra.Command{
	Use:     "list",
	GroupID: "info",
	Short:   "List all clusters",
	Long: `List all infrastructure clusters in your organization.

This command displays all clusters, showing their IDs, names, providers, and status.

Examples:
  agentuity cluster list
  agentuity cluster ls --format json`,
	Aliases: []string{"ls"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		format, _ := cmd.Flags().GetString("format")
		if format != "" {
			if err := validateFormat(format); err != nil {
				errsystem.New(errsystem.ErrInvalidArgumentProvided, err, errsystem.WithContextMessage("Invalid output format")).ShowErrorAndExit()
			}
		}

		var clusters []infrastructure.Cluster

		tui.ShowSpinner("Fetching clusters...", func() {
			var err error
			clusters, err = infrastructure.ListClusters(ctx, logger, apiUrl, apikey)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list clusters")).ShowErrorAndExit()
			}
		})

		if format == "json" {
			outputJSON(clusters)
			return
		}

		if len(clusters) == 0 {
			fmt.Println()
			tui.ShowWarning("no clusters found")
			fmt.Println()
			tui.ShowBanner("Create a new cluster", tui.Text("Use the ")+tui.Command("new")+tui.Text(" command to create a new cluster"), false)
			return
		}

		// Sort clusters by name
		sort.Slice(clusters, func(i, j int) bool {
			return clusters[i].Name < clusters[j].Name
		})

		headers := []string{
			tui.Title("ID"),
			tui.Title("Name"),
			tui.Title("Provider"),
			tui.Title("Size"),
			tui.Title("Region"),
			tui.Title("Created"),
		}

		rows := [][]string{}
		for _, cluster := range clusters {
			// Since backend doesn't have status or machine_count, we'll show type and created date
			rows = append(rows, []string{
				tui.Muted(cluster.ID),
				tui.Bold(cluster.Name),
				tui.Text(cluster.Provider),
				tui.Text(cluster.Type), // backend field name
				tui.Text(cluster.Region),
				tui.Muted(cluster.CreatedAt[:10]), // show date only
			})
		}

		tui.Table(headers, rows)

	},
}

var clusterRemoveCmd = &cobra.Command{
	Use:     "remove [id]",
	GroupID: "management",
	Short:   "Remove a cluster",
	Long: `Remove an infrastructure cluster by ID.

This command will delete the specified cluster and all its resources.

Arguments:
  [id]    The ID of the cluster to remove

Examples:
  agentuity cluster remove cluster-001
  agentuity cluster rm cluster-001 --force`,
	Aliases: []string{"rm", "del"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		clusterID := args[0]
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			if !tui.Ask(logger, fmt.Sprintf("Are you sure you want to remove cluster %s? This action cannot be undone.", clusterID), false) {
				tui.ShowWarning("cancelled")
				return
			}
		}

		tui.ShowSpinner(fmt.Sprintf("Removing cluster %s...", clusterID), func() {
			if err := infrastructure.DeleteCluster(ctx, logger, apiUrl, apikey, clusterID); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to remove cluster")).ShowErrorAndExit()
			}
		})

		tui.ShowSuccess("Cluster %s removed successfully", clusterID)

	},
}

var clusterStatusCmd = &cobra.Command{
	Use:     "status [id]",
	GroupID: "info",
	Short:   "Get cluster status",
	Long: `Get the detailed status of a specific cluster.

Arguments:
  [id]    The ID of the cluster

Examples:
  agentuity cluster status cluster-001
  agentuity cluster status cluster-001 --format json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		clusterID := args[0]
		format, _ := cmd.Flags().GetString("format")

		if format != "" {
			if err := validateFormat(format); err != nil {
				errsystem.New(errsystem.ErrInvalidArgumentProvided, err, errsystem.WithContextMessage("Invalid output format")).ShowErrorAndExit()
			}
		}

		var cluster *infrastructure.Cluster

		tui.ShowSpinner(fmt.Sprintf("Fetching cluster %s status...", clusterID), func() {
			var err error
			cluster, err = infrastructure.GetCluster(ctx, logger, apiUrl, apikey, clusterID)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get cluster status")).ShowErrorAndExit()
			}
		})

		if format == "json" {
			outputJSON(cluster)
			return
		}

		fmt.Printf("Cluster ID: %s\n", tui.Bold(cluster.ID))
		fmt.Printf("Name: %s\n", cluster.Name)
		fmt.Printf("Provider: %s\n", cluster.Provider)
		fmt.Printf("Size: %s\n", cluster.Type) // backend field is "type"
		fmt.Printf("Region: %s\n", cluster.Region)
		if cluster.OrgID != nil {
			fmt.Printf("Organization ID: %s\n", *cluster.OrgID)
		}
		if cluster.OrgName != nil {
			fmt.Printf("Organization: %s\n", *cluster.OrgName)
		}
		fmt.Printf("Created: %s\n", cluster.CreatedAt)
		if cluster.UpdatedAt != nil {
			fmt.Printf("Updated: %s\n", *cluster.UpdatedAt)
		}

	},
}

func init() {
	// Add command groups for cluster operations
	clusterCmd.AddGroup(&cobra.Group{
		ID:    "management",
		Title: "Cluster Management:",
	})
	clusterCmd.AddGroup(&cobra.Group{
		ID:    "info",
		Title: "Information:",
	})

	rootCmd.AddCommand(clusterCmd)
	clusterCmd.AddCommand(clusterNewCmd)
	clusterCmd.AddCommand(clusterListCmd)
	clusterCmd.AddCommand(clusterRemoveCmd)
	clusterCmd.AddCommand(clusterStatusCmd)

	// Flags for cluster new command
	clusterNewCmd.Flags().String("provider", "", "The infrastructure provider (gcp, aws, azure, vmware, other)")
	clusterNewCmd.Flags().String("size", "", "The cluster size (dev, small, medium, large)")
	clusterNewCmd.Flags().String("region", "", "The region to deploy the cluster")
	clusterNewCmd.Flags().String("format", "table", "Output format (table, json)")
	clusterNewCmd.Flags().String("org-id", "", "The organization to create the cluster in")

	// Flags for cluster list command
	clusterListCmd.Flags().String("format", "table", "Output format (table, json)")

	// Flags for cluster remove command
	clusterRemoveCmd.Flags().Bool("force", false, "Force removal without confirmation")

	// Flags for cluster status command
	clusterStatusCmd.Flags().String("format", "table", "Output format (table, json)")
}
