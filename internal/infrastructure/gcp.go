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

type gcpSetup struct {
}

var _ ClusterSetup = (*gcpSetup)(nil)

func (s *gcpSetup) Setup(ctx context.Context, logger logger.Logger, cluster *Cluster, format string) error {
	var canExecuteGCloud bool
	var projectName string
	var skipFailedDetection bool
	pubKey, privateKey, err := generateKey()
	if err != nil {
		return err
	}
	_, err = exec.LookPath("gcloud")
	if err == nil {
		val, err := runCommand(ctx, logger, "Checking gcloud account...", "gcloud", "config", "get-value", "project")
		if err == nil {
			canExecuteGCloud = true
			projectName = strings.TrimSpace(val)
			tui.ShowBanner("Google Cloud Tools Detected", "I’ll show you the command to run against the "+projectName+" gcloud project. You can choose to have me execute it for you, or run it yourself. If you prefer to run it on your own, the command will automatically be copied to your clipboard at each step.", false)
		} else {
			tui.ShowBanner("Google Cloud Tools Detected but not Authenticated", "I’ll show you the command to run against "+projectName+". You can choose to have me execute it for you, or run it yourself. If you prefer to run it on your own, the command will automatically be copied to your clipboard at each step.", false)
		}
		skipFailedDetection = true
	}
	if !skipFailedDetection {
		var defaultVal string
		if val, ok := os.LookupEnv("GOOGLE_CLOUD_PROJECT"); ok {
			defaultVal = val
		}
		tui.ShowBanner("No Google Cloud Tools Detected", "I’ll show you the command to run the commands yourself to create the cluster. The command will automatically be copied to your clipboard at each step. Please run the command manually for each step.", false)
		projectName = tui.Input(logger, "Please enter your Google Cloud Project ID:", defaultVal)
	}
	serviceAccount := "agentuity-cluster-" + cluster.ID + "@" + projectName + ".iam.gserviceaccount.com"

	executionContext := ExecutionContext{
		Context:  ctx,
		Logger:   logger,
		Runnable: canExecuteGCloud,
		Environment: map[string]any{
			"GCP_PROJECT_NAME":       projectName,
			"GCP_SERVICE_ACCOUNT":    serviceAccount,
			"ENCRYPTION_PUBLIC_KEY":  pubKey,
			"ENCRYPTION_PRIVATE_KEY": privateKey,
			"CLUSTER_TOKEN":          cluster.Token,
			"CLUSTER_ID":             cluster.ID,
			"CLUSTER_NAME":           cluster.Name,
			"CLUSTER_TYPE":           cluster.Type,
			"CLUSTER_REGION":         cluster.Region,
			"ENCRYPTION_KEY_NAME":    "agentuity-private-key",
		},
	}

	steps := make([]ExecutionSpec, 0)

	if err := json.Unmarshal([]byte(gcpSpecification), &steps); err != nil {
		return fmt.Errorf("error unmarshalling json: %w", err)
	}

	for _, step := range steps {
		if err := step.Run(executionContext); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	register("gcp", &gcpSetup{})
}

var gcpSpecification = `[
  {
    "title": "Create a Service Account",
    "description": "This service account will be used to control access to resources in the Google Cloud Platform to your Agentuity Cluster.",
    "execute": {
      "message": "Creating service account...",
      "command": "gcloud",
      "arguments": [
        "iam",
        "service-accounts",
        "create",
        "agentuity-cluster-{CLUSTER_ID}",
        "--display-name",
        "Agentuity Cluster ({CLUSTER_NAME})"
      ],
      "validate": "agentuity-cluster-{CLUSTER_ID}",
      "success": "Service account created"
    },
    "skip_if": {
      "message": "Checking service account...",
      "command": "gcloud",
      "arguments": [
        "iam",
        "service-accounts",
        "list",
        "--filter",
        "email:${GCP_SERVICE_ACCOUNT}"
      ],
      "validate": "{CLUSTER_ID}@"
    }
  },
  {
    "title": "Create encryption key and store in Google Secret Manager",
    "description": "Create private key used to decrypt the agent deployment data in your Agentuity Cluster.",
    "execute": {
      "message": "Creating encryption key...",
      "command": "echo",
      "arguments": [
        "{ENCRYPTION_PRIVATE_KEY}",
        "|",
        "base64",
        "--decode",
        "|",
        "gcloud",
        "secrets",
        "create",
        "{ENCRYPTION_KEY_NAME}",
        "--replication-policy=automatic",
        "--data-file=-"
      ],
      "success": "Secret created",
      "validate": "{ENCRYPTION_KEY_NAME}"
    },
    "skip_if": {
      "message": "Checking secret...",
      "command": "gcloud",
      "arguments": [
        "secrets",
        "list",
        "--filter",
        "name:{ENCRYPTION_KEY_NAME}"
      ],
      "validate": "{ENCRYPTION_KEY_NAME}"
    }
  },
  {
    "title": "Grant service account access to the encryption key Secret",
    "description": "Grant access to the Service Account to read the encryption key in your Agentuity Cluster.",
    "execute": {
      "message": "Creating encryption key...",
      "command": "gcloud",
      "arguments": [
        "secrets",
        "add-iam-policy-binding",
        "{ENCRYPTION_KEY_NAME}",
        "--member",
        "serviceAccount:{GCP_SERVICE_ACCOUNT}",
        "--role",
        "roles/secretmanager.secretAccessor"
      ],
      "success": "Secret access granted"
    },
    "skip_if": {
      "message": "Checking service account access...",
      "command": "gcloud",
      "arguments": [
        "secrets",
        "get-iam-policy",
        "{ENCRYPTION_KEY_NAME}",
        "--flatten",
        "bindings[].members",
        "--format",
        "value(bindings.members)",
        "--filter",
        "bindings.role=roles/secretmanager.secretAccessor AND bindings.members=serviceAccount:{GCP_SERVICE_ACCOUNT}"
      ],
      "validate": "agentuity-cluster@"
    }
  },
  {
    "title": "Create the Cluster Node",
    "description": "Create a new cluster node instance and launch it.",
    "execute": {
      "message": "Creating node...",
      "command": "gcloud",
      "arguments": [
        "compute",
        "instances",
        "create",
        "agentuity-node-cfd688",
        "--image-family",
        "hadron",
        "--image-project",
        "agentuity-stable",
        "--machine-type",
        "e2-standard-4",
        "--zone",
        "us-central1-a",
        "--subnet",
        "default",
        "--scopes",
        "https://www.googleapis.com/auth/cloud-platform",
        "--service-account",
        "{GCP_SERVICE_ACCOUNT}",
        "--metadata=user-data={CLUSTER_TOKEN}"
      ],
      "validate": "agentuity-node-cfd688",
      "success": "Node created"
    }
  }
]`
