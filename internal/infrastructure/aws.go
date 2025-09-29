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

type awsSetup struct{}

var _ ClusterSetup = (*awsSetup)(nil)

func (s *awsSetup) Setup(ctx context.Context, logger logger.Logger, cluster *Cluster, format string) error {
	var canExecuteAWS bool
	var region string
	pubKey, privateKey, err := generateKey()
	if err != nil {
		return err
	}

	// Check if AWS CLI is available and authenticated
	canExecuteAWS, region, err = s.canExecute(ctx, logger)
	if err != nil {
		return err
	}

	// Generate unique names for AWS resources
	roleName := "agentuity-cluster-" + cluster.ID
	policyName := "agentuity-cluster-policy-" + cluster.ID
	secretName := "agentuity-private-key-" + cluster.ID

	envs := map[string]any{
		"AWS_REGION":             region,
		"AWS_ROLE_NAME":          roleName,
		"AWS_POLICY_NAME":        policyName,
		"AWS_SECRET_NAME":        secretName,
		"ENCRYPTION_PUBLIC_KEY":  pubKey,
		"ENCRYPTION_PRIVATE_KEY": privateKey,
		"CLUSTER_TOKEN":          cluster.Token,
		"CLUSTER_ID":             cluster.ID,
		"CLUSTER_NAME":           cluster.Name,
		"CLUSTER_TYPE":           cluster.Type,
		"CLUSTER_REGION":         cluster.Region,
	}

	steps := make([]ExecutionSpec, 0)

	if err := json.Unmarshal([]byte(getAWSClusterSpecification(envs)), &steps); err != nil {
		return fmt.Errorf("error unmarshalling json: %w", err)
	}

	executionContext := ExecutionContext{
		Context:     ctx,
		Logger:      logger,
		Runnable:    canExecuteAWS,
		Environment: envs,
	}

	for _, step := range steps {
		if err := step.Run(executionContext); err != nil {
			return fmt.Errorf("failed at step '%s': %w", step.Title, err)
		}
	}

	tui.ShowSuccess("AWS infrastructure setup completed successfully!")
	return nil
}

func (s *awsSetup) CreateMachine(ctx context.Context, logger logger.Logger, region string, token string, clusterID string) error {
	// Need: {AWS_REGION} {AWS_ROLE_NAME} {CLUSTER_TOKEN} {AWS_INSTANCE_NAME} {CLUSTER_ID}

	roleName := "agentuity-cluster-" + clusterID
	instanceName := generateNodeName("agentuity-node")

	envs := map[string]any{
		"AWS_REGION":        region,
		"AWS_ROLE_NAME":     roleName,
		"CLUSTER_TOKEN":     token,
		"AWS_INSTANCE_NAME": instanceName,
		"CLUSTER_ID":        clusterID,
	}
	var steps []ExecutionSpec
	if err := json.Unmarshal([]byte(getAWSMachineSpecification(envs)), &steps); err != nil {
		return fmt.Errorf("error unmarshalling json: %w", err)
	}

	canExecuteAWS, _, err := s.canExecute(ctx, logger)
	if err != nil {
		return err
	}

	executionContext := ExecutionContext{
		Context:     ctx,
		Logger:      logger,
		Runnable:    canExecuteAWS,
		Environment: envs,
	}

	for _, step := range steps {
		if err := step.Run(executionContext); err != nil {
			return fmt.Errorf("failed at step '%s': %w", step.Title, err)
		}
	}
	return nil
}

func (s *awsSetup) canExecute(ctx context.Context, logger logger.Logger) (bool, string, error) {

	var canExecuteAWS bool
	var region string
	var skipFailedDetection bool
	var err error
	_, err = exec.LookPath("aws")
	if err == nil {
		_, err := runCommand(ctx, logger, "Checking AWS authentication...", "aws", "sts", "get-caller-identity")
		authenticated := err == nil
		if authenticated {
			val, err := runCommand(ctx, logger, "Checking AWS region...", "aws", "configure", "get", "region")
			if err == nil {
				canExecuteAWS = true
				region = strings.TrimSpace(val)
				if region == "" {
					region = "us-east-1" // default region
				}
				tui.ShowBanner("AWS Tools Detected", "I'll show you the command to run against the AWS account in region "+region+". You can choose to have me execute it for you, or run it yourself. If you prefer to run it on your own, the command will automatically be copied to your clipboard at each step.", false)
			}
		}
		if !canExecuteAWS && region != "" {
			tui.ShowBanner("AWS Tools Detected but not Authenticated", "I'll show you the command to run against AWS region "+region+". You can choose to have me execute it for you, or run it yourself. If you prefer to run it on your own, the command will automatically be copied to your clipboard at each step.", false)
		}
		skipFailedDetection = true
	}
	if !skipFailedDetection {
		var defaultVal string
		if val, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
			defaultVal = val
		} else if val, ok := os.LookupEnv("AWS_REGION"); ok {
			defaultVal = val
		}
		tui.ShowBanner("No AWS Tools Detected", "I'll show you the command to run the commands yourself to create the cluster. The command will automatically be copied to your clipboard at each step. Please run the command manually for each step.", false)
		region = tui.Input(logger, "Please enter your AWS region:", defaultVal)
		if region == "" {
			region = "us-east-1"
		}
	}

	return canExecuteAWS, region, nil
}

// Bash script functions removed - back to using ExecutionSpec array approach

func init() {
	register("aws", &awsSetup{})
}

var awsMachineSpecification = `[
  {
    "title": "Create the Cluster Node",
    "description": "Create a new cluster node instance and launch it.",
    "execute": {
      "message": "Creating node...",
      "command": "sh",
      "arguments": [
        "-c",
        "AMI_ID=$(aws ec2 describe-images --owners 084828583931 --filters 'Name=name,Values=hadron-*' 'Name=state,Values=available' --region {AWS_REGION} --query 'Images | sort_by(@, &CreationDate) | [-1].ImageId' --output text) && if [ \"$AMI_ID\" = \"\" ] || [ \"$AMI_ID\" = \"None\" ]; then SOURCE_AMI=$(aws ec2 describe-images --owners 084828583931 --filters 'Name=name,Values=hadron-*' 'Name=state,Values=available' --region us-west-1 --query 'Images | sort_by(@, &CreationDate) | [-1].ImageId' --output text) && AMI_ID=$(aws ec2 copy-image --source-image-id $SOURCE_AMI --source-region us-west-1 --region {AWS_REGION} --name \"hadron-copied-$(date +%s)\" --query 'ImageId' --output text) && aws ec2 wait image-available --image-ids $AMI_ID --region {AWS_REGION} && aws ec2 modify-image-attribute --image-id $AMI_ID --launch-permission 'Add=[{Group=all}]' --region {AWS_REGION}; fi && SUBNET_ID=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --region {AWS_REGION} --query 'Vpcs[0].VpcId' --output text | xargs -I {} aws ec2 describe-subnets --filters Name=vpc-id,Values={} Name=default-for-az,Values=true --region {AWS_REGION} --query 'Subnets[0].SubnetId' --output text) && SG_ID=$(aws ec2 describe-security-groups --filters Name=group-name,Values={AWS_ROLE_NAME}-sg --region {AWS_REGION} --query 'SecurityGroups[0].GroupId' --output text) && aws ec2 run-instances --image-id $AMI_ID --count 1 --instance-type t3.medium --security-group-ids $SG_ID --subnet-id $SUBNET_ID --iam-instance-profile Name={AWS_ROLE_NAME} --user-data '{CLUSTER_TOKEN}' --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value={AWS_INSTANCE_NAME}},{Key=AgentuityCluster,Value={CLUSTER_ID}}]' --region {AWS_REGION}"
      ],
      "validate": "{AWS_INSTANCE_NAME}",
      "success": "Node created"
    }
  }
]`

var awsClusterSpecification = `[
  {
    "title": "Create IAM Role for Agentuity Cluster",
    "description": "This IAM role will be used to control access to AWS resources for your Agentuity Cluster.",
    "execute": {
      "message": "Creating IAM role...",
      "command": "aws",
      "arguments": [
        "iam",
        "create-role",
        "--role-name",
        "{AWS_ROLE_NAME}",
        "--assume-role-policy-document",
        "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":\"ec2.amazonaws.com\"},\"Action\":\"sts:AssumeRole\"}]}"
      ],
      "validate": "{AWS_ROLE_NAME}",
      "success": "IAM role created"
    },
    "skip_if": {
      "message": "Checking IAM role...",
      "command": "aws",
      "arguments": [
        "iam",
        "get-role",
        "--role-name",
        "{AWS_ROLE_NAME}"
      ],
      "validate": "{AWS_ROLE_NAME}"
    }
  },
  {
    "title": "Create IAM Policy for Agentuity Cluster",
    "description": "This policy grants the necessary permissions for the Agentuity Cluster to access AWS services.",
    "execute": {
      "message": "Creating IAM policy...",
      "command": "aws",
      "arguments": [
        "iam",
        "create-policy",
        "--policy-name",
        "{AWS_POLICY_NAME}",
        "--policy-document",
        "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"secretsmanager:GetSecretValue\",\"secretsmanager:DescribeSecret\"],\"Resource\":\"arn:aws:secretsmanager:{AWS_REGION}:*:secret:{AWS_SECRET_NAME}*\"},{\"Effect\":\"Allow\",\"Action\":[\"ec2:DescribeInstances\",\"ec2:DescribeTags\"],\"Resource\":\"*\"}]}"
      ],
      "validate": "{AWS_POLICY_NAME}",
      "success": "IAM policy created"
    },
    "skip_if": {
      "message": "Checking IAM policy...",
      "command": "aws",
      "arguments": [
        "iam",
        "list-policies",
        "--query",
        "Policies[?PolicyName=='{AWS_POLICY_NAME}'].PolicyName",
        "--output",
        "text"
      ],
      "validate": "{AWS_POLICY_NAME}"
    }
  },
  {
    "title": "Attach Policy to IAM Role",
    "description": "Attach the Agentuity policy to the IAM role so the cluster can access the required resources.",
    "execute": {
      "message": "Attaching policy to role...",
      "command": "sh",
      "arguments": [
        "-c",
        "aws iam attach-role-policy --role-name {AWS_ROLE_NAME} --policy-arn arn:aws:iam::$(aws sts get-caller-identity --query Account --output text):policy/{AWS_POLICY_NAME}"
      ],
      "success": "Policy attached to role"
    },
    "skip_if": {
      "message": "Checking policy attachment...",
      "command": "aws",
      "arguments": [
        "iam",
        "list-attached-role-policies",
        "--role-name",
        "{AWS_ROLE_NAME}",
        "--query",
        "AttachedPolicies[?PolicyName=='{AWS_POLICY_NAME}'].PolicyName",
        "--output",
        "text"
      ],
      "validate": "{AWS_POLICY_NAME}"
    }
  },
  {
    "title": "Create Instance Profile",
    "description": "Create an instance profile to attach the IAM role to EC2 instances.",
    "execute": {
      "message": "Creating instance profile...",
      "command": "aws",
      "arguments": [
        "iam",
        "create-instance-profile",
        "--instance-profile-name",
        "{AWS_ROLE_NAME}"
      ],
      "validate": "{AWS_ROLE_NAME}",
      "success": "Instance profile created"
    },
    "skip_if": {
      "message": "Checking instance profile...",
      "command": "aws",
      "arguments": [
        "iam",
        "get-instance-profile",
        "--instance-profile-name",
        "{AWS_ROLE_NAME}"
      ],
      "validate": "{AWS_ROLE_NAME}"
    }
  },
  {
    "title": "Add Role to Instance Profile",
    "description": "Add the IAM role to the instance profile so it can be used by EC2 instances.",
    "execute": {
      "message": "Adding role to instance profile...",
      "command": "aws",
      "arguments": [
        "iam",
        "add-role-to-instance-profile",
        "--instance-profile-name",
        "{AWS_ROLE_NAME}",
        "--role-name",
        "{AWS_ROLE_NAME}"
      ],
      "success": "Role added to instance profile"
    },
    "skip_if": {
      "message": "Checking role in instance profile...",
      "command": "aws",
      "arguments": [
        "iam",
        "get-instance-profile",
        "--instance-profile-name",
        "{AWS_ROLE_NAME}",
        "--query",
        "InstanceProfile.Roles[?RoleName=='{AWS_ROLE_NAME}'].RoleName",
        "--output",
        "text"
      ],
      "validate": "{AWS_ROLE_NAME}"
    }
  },
  {
    "title": "Create encryption key and store in AWS Secrets Manager",
    "description": "Create private key used to decrypt the agent deployment data in your Agentuity Cluster.",
    "execute": {
      "message": "Creating encryption key...",
      "command": "sh",
      "arguments": [
        "-c",
        "echo '{ENCRYPTION_PRIVATE_KEY}' | base64 -d | aws secretsmanager create-secret --name '{AWS_SECRET_NAME}' --description 'Agentuity Cluster Private Key' --secret-binary fileb://-"
      ],
      "success": "Secret created",
      "validate": "{AWS_SECRET_NAME}"
    },
    "skip_if": {
      "message": "Checking secret...",
      "command": "aws",
      "arguments": [
        "secretsmanager",
        "describe-secret",
        "--secret-id",
        "{AWS_SECRET_NAME}"
      ],
      "validate": "{AWS_SECRET_NAME}"
    }
  },
  {
    "title": "Get Default VPC",
    "description": "Find the default VPC to use for the cluster node.",
    "execute": {
      "message": "Finding default VPC...",
      "command": "aws",
      "arguments": [
        "ec2",
        "describe-vpcs",
        "--filters",
        "Name=isDefault,Values=true",
        "--query",
        "Vpcs[0].VpcId",
        "--output",
        "text"
      ],
      "success": "Found default VPC"
    }
  },
  {
    "title": "Get Default Subnet",
    "description": "Find a default subnet in the default VPC.",
    "execute": {
      "message": "Finding default subnet...",
      "command": "sh",
      "arguments": [
        "-c",
        "VPC_ID=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text) && aws ec2 describe-subnets --filters Name=vpc-id,Values=$VPC_ID Name=default-for-az,Values=true --query 'Subnets[0].SubnetId' --output text"
      ],
      "success": "Found default subnet"
    }
  },
  {
    "title": "Create Security Group",
    "description": "Create a security group for the Agentuity cluster with necessary ports.",
    "execute": {
      "message": "Creating security group...",
      "command": "sh",
      "arguments": [
        "-c",
        "VPC_ID=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text) && aws ec2 create-security-group --group-name {AWS_ROLE_NAME}-sg --description 'Agentuity Cluster Security Group' --vpc-id $VPC_ID --query 'GroupId' --output text"
      ],
      "success": "Security group created"
    },
    "skip_if": {
      "message": "Checking security group...",
      "command": "aws",
      "arguments": [
        "ec2",
        "describe-security-groups",
        "--filters",
        "Name=group-name,Values={AWS_ROLE_NAME}-sg",
        "--query",
        "SecurityGroups[0].GroupId",
        "--output",
        "text"
      ],
      "validate": "sg-"
    }
  },
  {
    "title": "Configure Security Group Rules",
    "description": "Allow SSH and HTTPS traffic for the cluster.",
    "execute": {
      "message": "Configuring security group rules...",
      "command": "sh",
      "arguments": [
        "-c",
        "SG_ID=$(aws ec2 describe-security-groups --filters Name=group-name,Values={AWS_ROLE_NAME}-sg --query 'SecurityGroups[0].GroupId' --output text) && aws ec2 authorize-security-group-ingress --group-id $SG_ID --protocol tcp --port 22 --cidr 0.0.0.0/0 2>/dev/null || true && aws ec2 authorize-security-group-ingress --group-id $SG_ID --protocol tcp --port 443 --cidr 0.0.0.0/0 2>/dev/null || true"
      ],
      "success": "Security group configured"
    },
    "skip_if": {
      "message": "Checking security group rules...",
      "command": "sh",
      "arguments": [
        "-c",
        "SG_ID=$(aws ec2 describe-security-groups --filters Name=group-name,Values={AWS_ROLE_NAME}-sg --query 'SecurityGroups[0].GroupId' --output text) && aws ec2 describe-security-groups --group-ids $SG_ID --query 'SecurityGroups[0].IpPermissions[?FromPort==\"22\"]' --output text"
      ],
      "validate": "22"
    }
  }
]`

func getAWSClusterSpecification(envs map[string]any) string {
	spec := awsClusterSpecification

	// Replace variables in the JSON string
	for key, val := range envs {
		spec = strings.ReplaceAll(spec, "{"+key+"}", fmt.Sprint(val))
	}

	return spec
}

func getAWSMachineSpecification(envs map[string]any) string {
	spec := awsMachineSpecification

	// Replace variables in the JSON string
	for key, val := range envs {
		spec = strings.ReplaceAll(spec, "{"+key+"}", fmt.Sprint(val))
	}

	return spec
}
