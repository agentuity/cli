package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

type azureSetup struct{}

var _ ClusterSetup = (*azureSetup)(nil)

func (s *azureSetup) Setup(ctx context.Context, logger logger.Logger, cluster *Cluster, format string) error {
	var canExecuteAzure bool
	var subscriptionID string
	var resourceGroup string
	var region string
	pubKey, privateKey, err := generateKey()
	if err != nil {
		return err
	}

	// Check if Azure CLI is available and authenticated
	canExecuteAzure, subscriptionID, resourceGroup, _, err = s.canExecute(ctx, logger)
	if err != nil {
		return err
	}

	// Always use the cluster region first, fall back to detected/default only if not specified
	region = cluster.Region
	fmt.Println("Region: ", region)
	if region == "" {
		// Try to get default location from Azure CLI, fall back to eastus
		if loc, err := runCommand(ctx, logger, "Getting default location...", "az", "configure", "get", "location"); err == nil && strings.TrimSpace(loc) != "" {
			region = strings.TrimSpace(loc)
		} else {
			region = "eastus" // final fallback
		}
	}
	// Generate unique names for Azure resources
	servicePrincipalName := "agentuity-cluster-" + cluster.ID
	keyVaultName := "agentuity-kv-" + cluster.ID[len(cluster.ID)-6:]
	secretName := "agentuity-private-key"
	networkSecurityGroupName := "agentuity-nsg-" + cluster.ID

	envs := map[string]any{
		"AZURE_SUBSCRIPTION_ID":   subscriptionID,
		"AZURE_RESOURCE_GROUP":    resourceGroup,
		"AZURE_SERVICE_PRINCIPAL": servicePrincipalName,
		"AZURE_KEY_VAULT":         keyVaultName,
		"AZURE_SECRET_NAME":       secretName,
		"AZURE_NSG_NAME":          networkSecurityGroupName,
		"ENCRYPTION_PUBLIC_KEY":   pubKey,
		"ENCRYPTION_PRIVATE_KEY":  privateKey,
		"CLUSTER_TOKEN":           cluster.Token,
		"CLUSTER_ID":              cluster.ID,
		"CLUSTER_NAME":            cluster.Name,
		"CLUSTER_TYPE":            cluster.Type,
		"CLUSTER_REGION":          region,
		"AZURE_REGION":            region,
	}

	steps := make([]ExecutionSpec, 0)

	if err := json.Unmarshal([]byte(getAzureClusterSpecification(envs)), &steps); err != nil {
		return fmt.Errorf("error unmarshalling json: %w", err)
	}

	executionContext := ExecutionContext{
		Context:     ctx,
		Logger:      logger,
		Runnable:    canExecuteAzure,
		Environment: envs,
	}

	for _, step := range steps {
		if err := step.Run(executionContext); err != nil {
			return fmt.Errorf("failed at step '%s': %w", step.Title, err)
		}
	}

	tui.ShowSuccess("Azure infrastructure setup completed successfully!")
	return nil
}

func (s *azureSetup) CreateMachine(ctx context.Context, logger logger.Logger, region string, token string, clusterID string) error {
	// Get Azure context information
	canExecuteAzure, subscriptionID, resourceGroup, _, err := s.canExecute(ctx, logger)
	if err != nil {
		return err
	}

	servicePrincipalName := "agentuity-cluster-" + clusterID
	vmName := generateNodeName("agentuity-node")
	networkSecurityGroupName := "agentuity-nsg-" + clusterID
	keyVaultName := "agentuity-kv-" + clusterID[len(clusterID)-6:]

	envs := map[string]any{
		"AZURE_SUBSCRIPTION_ID":   subscriptionID,
		"AZURE_RESOURCE_GROUP":    resourceGroup,
		"AZURE_SERVICE_PRINCIPAL": servicePrincipalName,
		"AZURE_NSG_NAME":          networkSecurityGroupName,
		"AZURE_REGION":            region,
		"CLUSTER_TOKEN":           token,
		"AZURE_VM_NAME":           vmName,
		"CLUSTER_ID":              clusterID,
		"AZURE_KEY_VAULT":         keyVaultName,
	}

	var steps []ExecutionSpec
	if err := json.Unmarshal([]byte(getAzureMachineSpecification(envs)), &steps); err != nil {
		return fmt.Errorf("error unmarshalling json: %w", err)
	}

	// We already got canExecuteAzure above, so use it directly

	executionContext := ExecutionContext{
		Context:     ctx,
		Logger:      logger,
		Runnable:    canExecuteAzure,
		Environment: envs,
	}

	for _, step := range steps {
		if err := step.Run(executionContext); err != nil {
			return fmt.Errorf("failed at step '%s': %w", step.Title, err)
		}
	}
	return nil
}

func (s *azureSetup) canExecute(ctx context.Context, logger logger.Logger) (bool, string, string, string, error) {
	var canExecuteAzure bool
	var subscriptionID string
	var resourceGroup string
	var region string
	var skipFailedDetection bool
	var err error

	_, err = exec.LookPath("az")
	if err == nil {
		// Check if authenticated
		_, err := runCommand(ctx, logger, "Checking Azure authentication...", "az", "account", "show")
		authenticated := err == nil
		if authenticated {
			// Get subscription ID
			subID, err := runCommand(ctx, logger, "Getting Azure subscription...", "az", "account", "show", "--query", "id", "-o", "tsv")
			if err == nil {
				canExecuteAzure = true
				subscriptionID = strings.TrimSpace(subID)

				// Get default location
				if loc, err := runCommand(ctx, logger, "Getting default location...", "az", "configure", "get", "location"); err == nil && strings.TrimSpace(loc) != "" {
					region = strings.TrimSpace(loc)
				} else {
					region = "eastus" // default location
				}

				// Get or create resource group
				rgName := "agentuity-rg"
				if rgExists, _ := runCommand(ctx, logger, "Checking resource group...", "az", "group", "exists", "--name", rgName); strings.TrimSpace(rgExists) == "false" {
					tui.ShowBanner("Creating Resource Group", "Creating resource group "+rgName+" in "+region, false)
					runCommand(ctx, logger, "Creating resource group...", "az", "group", "create", "--name", rgName, "--location", region)
				}
				resourceGroup = rgName

				tui.ShowBanner("Azure Tools Detected", "I'll show you the command to run against Azure subscription "+subscriptionID+" in region "+region+". You can choose to have me execute it for you, or run it yourself. If you prefer to run it on your own, the command will automatically be copied to your clipboard at each step.", false)
			}
		}
		if !canExecuteAzure {
			tui.ShowBanner("Azure Tools Detected but not Authenticated", "I'll show you the commands to run against Azure. You can choose to have me execute them for you, or run them yourself. If you prefer to run them on your own, the commands will automatically be copied to your clipboard at each step.", false)
		}
		skipFailedDetection = true
	}

	if !skipFailedDetection {
		var defaultSubID string
		if val, ok := os.LookupEnv("AZURE_SUBSCRIPTION_ID"); ok {
			defaultSubID = val
		}
		tui.ShowBanner("No Azure Tools Detected", "I'll show you the commands to run manually to create the cluster. The commands will automatically be copied to your clipboard at each step. Please run each command manually.", false)
		subscriptionID = tui.Input(logger, "Please enter your Azure subscription ID:", defaultSubID)
		resourceGroup = tui.Input(logger, "Please enter your Azure resource group name:", "agentuity-rg")
		region = tui.Input(logger, "Please enter your Azure region:", "eastus")
	}

	return canExecuteAzure, subscriptionID, resourceGroup, region, nil
}

func init() {
	register("azure", &azureSetup{})
}

// Azure command functions
func azure_registerProviders() string {
	cmd := []string{
		`az provider register --namespace Microsoft.KeyVault`,
		`az provider register --namespace Microsoft.Compute`,
		`az provider register --namespace Microsoft.Network`,
		`timeout 300 bash -c 'until [ "$(az provider show --namespace Microsoft.KeyVault --query registrationState -o tsv)" = "Registered" ]; do echo "Still registering..."; sleep 10; done'`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_createServicePrincipal() string {
	cmd := []string{
		`az ad sp create-for-rbac --name {AZURE_SERVICE_PRINCIPAL} --role Contributor --scopes /subscriptions/{AZURE_SUBSCRIPTION_ID}/resourceGroups/{AZURE_RESOURCE_GROUP} --query "appId" -o tsv`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_checkServicePrincipal() string {
	cmd := []string{
		`az ad sp list --display-name {AZURE_SERVICE_PRINCIPAL} --query "[0].appId" -o tsv`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_assignKeyVaultRole() string {
	cmd := []string{
		`SP_APP_ID=$(az ad sp list --display-name {AZURE_SERVICE_PRINCIPAL} --query "[0].appId" -o tsv)`,
		`az role assignment create --role "Key Vault Secrets User" --assignee $SP_APP_ID --scope /subscriptions/{AZURE_SUBSCRIPTION_ID}/resourceGroups/{AZURE_RESOURCE_GROUP}/providers/Microsoft.KeyVault/vaults/{AZURE_KEY_VAULT}`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_createKeyVault() string {
	cmd := []string{
		`if az keyvault show --name {AZURE_KEY_VAULT} >/dev/null 2>&1; then echo "Key Vault exists, checking RBAC status..."; RBAC_ENABLED=$(az keyvault show --name {AZURE_KEY_VAULT} --query "properties.enableRbacAuthorization" -o tsv); if [ "$RBAC_ENABLED" = "true" ]; then echo "RBAC is enabled, deleting and recreating Key Vault..."; az keyvault delete --name {AZURE_KEY_VAULT} --resource-group {AZURE_RESOURCE_GROUP}; az keyvault purge --name {AZURE_KEY_VAULT} --location {CLUSTER_REGION}; sleep 30; fi; fi`,
		`az keyvault create --name {AZURE_KEY_VAULT} --resource-group {AZURE_RESOURCE_GROUP} --location {CLUSTER_REGION} --enable-rbac-authorization false --query "properties.vaultUri" -o tsv`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_checkKeyVault() string {
	cmd := []string{
		`VAULT_URI=$(az keyvault show --name {AZURE_KEY_VAULT} --query "properties.vaultUri" -o tsv 2>/dev/null)`,
		`RBAC_ENABLED=$(az keyvault show --name {AZURE_KEY_VAULT} --query "properties.enableRbacAuthorization" -o tsv 2>/dev/null)`,
		`if [ "$VAULT_URI" != "" ] && [ "$RBAC_ENABLED" = "false" ]; then echo "$VAULT_URI"; else echo ""; fi`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_createSecret() string {
	cmd := []string{
		`echo '{ENCRYPTION_PRIVATE_KEY}' | base64 -d > /tmp/agentuity-key.pem`,
		`az keyvault secret set --vault-name {AZURE_KEY_VAULT} --name {AZURE_SECRET_NAME} --file /tmp/agentuity-key.pem`,
		`rm -f /tmp/agentuity-key.pem`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_checkSecret() string {
	cmd := []string{
		`az keyvault secret show --vault-name {AZURE_KEY_VAULT} --name {AZURE_SECRET_NAME} --query "id" -o tsv`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_createNetworkSecurityGroup() string {
	cmd := []string{
		`az network nsg create --resource-group {AZURE_RESOURCE_GROUP} --name {AZURE_NSG_NAME} --location {CLUSTER_REGION}`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_checkNetworkSecurityGroup() string {
	cmd := []string{
		`az network nsg show --resource-group {AZURE_RESOURCE_GROUP} --name {AZURE_NSG_NAME} --query "id" -o tsv`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_configureSecurityGroupRules() string {
	cmd := []string{
		`az network nsg rule create --resource-group {AZURE_RESOURCE_GROUP} --nsg-name {AZURE_NSG_NAME} --name SSH --protocol tcp --priority 1000 --destination-port-range 22 --source-address-prefix '*' --destination-address-prefix '*' --access allow --direction inbound`,
		`az network nsg rule create --resource-group {AZURE_RESOURCE_GROUP} --nsg-name {AZURE_NSG_NAME} --name HTTPS --protocol tcp --priority 1010 --destination-port-range 443 --source-address-prefix '*' --destination-address-prefix '*' --access allow --direction inbound`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_checkSecurityGroupRules() string {
	cmd := []string{
		`az network nsg rule list --resource-group {AZURE_RESOURCE_GROUP} --nsg-name {AZURE_NSG_NAME} --query "[?destinationPortRange=='22']" -o tsv`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_validateInfrastructure() string {
	cmd := []string{
		`az network nsg list --resource-group {AZURE_RESOURCE_GROUP} --query "[].name" -o table || echo "No NSGs found or resource group doesn't exist"`,
		`az network nsg list --resource-group {AZURE_RESOURCE_GROUP} --query "[?contains(name, 'agentuity-nsg')]" -o table || echo "No agentuity NSGs found"`,
		`az network nsg show --resource-group {AZURE_RESOURCE_GROUP} --name {AZURE_NSG_NAME} --query "location" -o tsv`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_checkSSHKey() string {
	cmd := []string{
		`if [ ! -f ~/.ssh/id_rsa.pub ]; then echo "SSH key not found, generating new key pair..."; ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa -N "" -q; echo "SSH key generated at ~/.ssh/id_rsa"; else echo "SSH key found at ~/.ssh/id_rsa.pub"; fi`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_createVMOnly() string {
	cmd := []string{
		`IMAGE_ID=$(az image list --resource-group HADRON-IMAGES --query "[?starts_with(name, 'hadron-')] | sort_by(@, &tags.build_time) | [-1].id" -o tsv)`,
		`if [ "$IMAGE_ID" = "" ] || [ "$IMAGE_ID" = "null" ]; then echo "ERROR: No hadron images found!"; exit 1; fi`,
		`IMAGE_NAME=$(az image show --ids "$IMAGE_ID" --query "name" -o tsv)`,
		`NSG_LOCATION=$(az network nsg show --resource-group {AZURE_RESOURCE_GROUP} --name {AZURE_NSG_NAME} --query "location" -o tsv)`,
		`az vm create --resource-group {AZURE_RESOURCE_GROUP} --name {AZURE_VM_NAME} --image "$IMAGE_ID" --plan-name "9-base" --plan-product "rockylinux-x86_64" --plan-publisher "resf" --admin-username rocky --ssh-key-values ~/.ssh/id_rsa.pub --authentication-type ssh --size Standard_D2s_v3 --location "$NSG_LOCATION" --nsg {AZURE_NSG_NAME} --assign-identity --role "Reader" --scope /subscriptions/{AZURE_SUBSCRIPTION_ID}/resourceGroups/{AZURE_RESOURCE_GROUP} --tags AgentuityCluster={CLUSTER_ID} --user-data={CLUSTER_TOKEN}`,
		`VM_IDENTITY=$(az vm show --resource-group {AZURE_RESOURCE_GROUP} --name {AZURE_VM_NAME} --query "identity.principalId" -o tsv)`,
		`az keyvault set-policy --name {AZURE_KEY_VAULT} --object-id $VM_IDENTITY --secret-permissions get list`,
	}
	return azure_cmdEscape(strings.Join(cmd, " && "))
}

func azure_cmdEscape(cmd string) string {
	return strings.ReplaceAll(strings.ReplaceAll(cmd, `\`, `\\`), `"`, `\"`)
}

func azureMachineSpecification() string {
	return `[
  {
    "title": "Validate Cluster Infrastructure",
    "description": "Check that the cluster's network security group exists and get its region.",
    "execute": {
      "message": "Validating cluster infrastructure...",
      "command": "sh",
      "arguments": [
        "-c", "` + azure_validateInfrastructure() + `"
      ],
      "success": "Infrastructure validated"
    }
  },
  {
    "title": "Check SSH Key",
    "description": "Verify SSH key exists or generate a new one for VM access.",
    "execute": {
      "message": "Checking SSH key...",
      "command": "sh",
      "arguments": [
        "-c", "` + azure_checkSSHKey() + `"
      ],
      "success": "SSH key ready"
    }
  },
  {
    "title": "Deploy Virtual Machine",
    "description": "Create the VM with selected image and cluster configuration.",
    "execute": {
      "message": "Deploying VM...",
      "command": "sh",
      "arguments": [
        "-c", "` + azure_createVMOnly() + `"
      ],
      "validate": "{AZURE_VM_NAME}",
      "success": "VM deployed successfully"
    }
  }
]`
}

var azureClusterSpecification = `[
  {
    "title": "Register Azure Resource Providers",
    "description": "Register the required Azure resource providers for Key Vault, Compute, and Network services.",
    "execute": {
      "message": "Registering Azure resource providers...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_registerProviders() + `" ],
      "success": "Resource providers registered"
    }
  },
  {
    "title": "Create Service Principal for Agentuity Cluster",
    "description": "This service principal will be used to control access to Azure resources for your Agentuity Cluster.",
    "execute": {
      "message": "Creating service principal...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_createServicePrincipal() + `" ],
      "validate": "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}",
      "success": "Service principal created"
    },
    "skip_if": {
      "message": "Checking service principal...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_checkServicePrincipal() + `" ],
      "validate": "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
    }
  },
  {
    "title": "Create Key Vault for Encryption Keys",
    "description": "Create an Azure Key Vault to securely store the encryption keys.",
    "execute": {
      "message": "Creating Key Vault...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_createKeyVault() + `" ],
      "validate": "https://",
      "success": "Key Vault created"
    },
    "skip_if": {
      "message": "Checking Key Vault...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_checkKeyVault() + `" ],
      "validate": "https://"
    }
  },
  {
  "title": "Assign Key Vault Role to Service Principal",
  "description": "Assign the Key Vault Secrets User role to the service principal for accessing secrets.",
  "execute": {
  "message": "Assigning Key Vault role...",
  "command": "sh",
  "arguments": [ "-c", "` + azure_assignKeyVaultRole() + `" ],
  "success": "Key Vault role assigned"
  }
  },
  {
    "title": "Create encryption key and store in Azure Key Vault",
    "description": "Create private key used to decrypt the agent deployment data in your Agentuity Cluster.",
    "execute": {
      "message": "Creating encryption key...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_createSecret() + `" ],
      "success": "Secret created",
      "validate": "{AZURE_SECRET_NAME}"
    },
    "skip_if": {
      "message": "Checking secret...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_checkSecret() + `" ],
      "validate": "{AZURE_SECRET_NAME}"
    }
  },
  {
    "title": "Create Network Security Group",
    "description": "Create a network security group for the Agentuity cluster with necessary ports.",
    "execute": {
      "message": "Creating network security group...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_createNetworkSecurityGroup() + `" ],
      "success": "Network security group created"
    },
    "skip_if": {
      "message": "Checking network security group...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_checkNetworkSecurityGroup() + `" ],
      "validate": "/networkSecurityGroups/"
    }
  },
  {
    "title": "Configure Network Security Group Rules",
    "description": "Allow SSH and HTTPS traffic for the cluster.",
    "execute": {
      "message": "Configuring security group rules...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_configureSecurityGroupRules() + `" ],
      "success": "Security group configured"
    },
    "skip_if": {
      "message": "Checking security group rules...",
      "command": "sh",
      "arguments": [ "-c", "` + azure_checkSecurityGroupRules() + `" ],
      "validate": "22"
    }
  }
]`

func getAzureClusterSpecification(envs map[string]any) string {
	spec := azureClusterSpecification
	// Replace variables in the JSON string
	for key, val := range envs {
		spec = strings.ReplaceAll(spec, "{"+key+"}", fmt.Sprint(val))
	}
	return spec
}

func getAzureMachineSpecification(envs map[string]any) string {
	spec := azureMachineSpecification()
	// Replace variables in the JSON string
	for key, val := range envs {
		spec = strings.ReplaceAll(spec, "{"+key+"}", fmt.Sprint(val))
	}
	return spec
}
