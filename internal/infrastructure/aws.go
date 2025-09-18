package infrastructure

import (
	"context"
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
	var skipFailedDetection bool
	pubKey, privateKey, err := generateKey()
	if err != nil {
		return err
	}

	// Check if AWS CLI is available and authenticated
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

	// Generate unique names for AWS resources
	roleName := "agentuity-cluster-" + cluster.ID
	policyName := "agentuity-cluster-policy-" + cluster.ID
	secretName := "agentuity-private-key-" + cluster.ID
	instanceName := generateNodeName("agentuity-node")

	envs := map[string]any{
		"AWS_REGION":             region,
		"AWS_ROLE_NAME":          roleName,
		"AWS_POLICY_NAME":        policyName,
		"AWS_SECRET_NAME":        secretName,
		"AWS_INSTANCE_NAME":      instanceName,
		"ENCRYPTION_PUBLIC_KEY":  pubKey,
		"ENCRYPTION_PRIVATE_KEY": privateKey,
		"CLUSTER_TOKEN":          cluster.Token,
		"CLUSTER_ID":             cluster.ID,
		"CLUSTER_NAME":           cluster.Name,
		"CLUSTER_TYPE":           cluster.Type,
		"CLUSTER_REGION":         cluster.Region,
	}

	steps := getAWSSpecification(envs)

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

// Bash script functions removed - back to using ExecutionSpec array approach

func init() {
	register("aws", &awsSetup{})
}

// Legacy function - keeping for potential future use
func getAWSSpecification(envs map[string]any) []ExecutionSpec {
	spec := []ExecutionSpec{
		{
			Title:       "Create IAM Role for Agentuity Cluster",
			Description: "This IAM role will be used to control access to AWS resources for your Agentuity Cluster.",
			Execute: ExecutionCommand{
				Message: "Creating IAM role...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"create-role",
					"--role-name",
					"{AWS_ROLE_NAME}",
					"--assume-role-policy-document",
					"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":\"ec2.amazonaws.com\"},\"Action\":\"sts:AssumeRole\"}]}",
				},
				Validate: Validation("{AWS_ROLE_NAME}"),
				Success:  "IAM role created",
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking IAM role...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"get-role",
					"--role-name",
					"{AWS_ROLE_NAME}",
				},
				Validate: Validation("{AWS_ROLE_NAME}"),
			},
		},
		{
			Title:       "Create IAM Policy for Agentuity Cluster",
			Description: "This policy grants the necessary permissions for the Agentuity Cluster to access AWS services.",
			Execute: ExecutionCommand{
				Message: "Creating IAM policy...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"create-policy",
					"--policy-name",
					"{AWS_POLICY_NAME}",
					"--policy-document",
					"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"secretsmanager:GetSecretValue\",\"secretsmanager:DescribeSecret\"],\"Resource\":\"arn:aws:secretsmanager:{AWS_REGION}:*:secret:{AWS_SECRET_NAME}*\"},{\"Effect\":\"Allow\",\"Action\":[\"ec2:DescribeInstances\",\"ec2:DescribeTags\"],\"Resource\":\"*\"}]}",
				},
				Validate: Validation("{AWS_POLICY_NAME}"),
				Success:  "IAM policy created",
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking IAM policy...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"list-policies",
					"--query",
					"Policies[?PolicyName=='{AWS_POLICY_NAME}'].PolicyName",
					"--output",
					"text",
				},
				Validate: Validation("{AWS_POLICY_NAME}"),
			},
		},
		{
			Title:       "Attach Policy to IAM Role",
			Description: "Attach the Agentuity policy to the IAM role so the cluster can access the required resources.",
			Execute: ExecutionCommand{
				Message: "Attaching policy to role...",
				Command: "sh",
				Arguments: []string{
					"-c",
					"aws iam attach-role-policy --role-name {AWS_ROLE_NAME} --policy-arn arn:aws:iam::$(aws sts get-caller-identity --query Account --output text):policy/{AWS_POLICY_NAME}",
				},
				Success: "Policy attached to role",
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking policy attachment...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"list-attached-role-policies",
					"--role-name",
					"{AWS_ROLE_NAME}",
					"--query",
					"AttachedPolicies[?PolicyName=='{AWS_POLICY_NAME}'].PolicyName",
					"--output",
					"text",
				},
				Validate: Validation("{AWS_POLICY_NAME}"),
			},
		},
		{
			Title:       "Create Instance Profile",
			Description: "Create an instance profile to attach the IAM role to EC2 instances.",
			Execute: ExecutionCommand{
				Message: "Creating instance profile...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"create-instance-profile",
					"--instance-profile-name",
					"{AWS_ROLE_NAME}",
				},
				Validate: Validation("{AWS_ROLE_NAME}"),
				Success:  "Instance profile created",
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking instance profile...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"get-instance-profile",
					"--instance-profile-name",
					"{AWS_ROLE_NAME}",
				},
				Validate: Validation("{AWS_ROLE_NAME}"),
			},
		},
		{
			Title:       "Add Role to Instance Profile",
			Description: "Add the IAM role to the instance profile so it can be used by EC2 instances.",
			Execute: ExecutionCommand{
				Message: "Adding role to instance profile...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"add-role-to-instance-profile",
					"--instance-profile-name",
					"{AWS_ROLE_NAME}",
					"--role-name",
					"{AWS_ROLE_NAME}",
				},
				Success: "Role added to instance profile",
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking role in instance profile...",
				Command: "aws",
				Arguments: []string{
					"iam",
					"get-instance-profile",
					"--instance-profile-name",
					"{AWS_ROLE_NAME}",
					"--query",
					"InstanceProfile.Roles[?RoleName=='{AWS_ROLE_NAME}'].RoleName",
					"--output",
					"text",
				},
				Validate: Validation("{AWS_ROLE_NAME}"),
			},
		},
		{
			Title:       "Create encryption key and store in AWS Secrets Manager",
			Description: "Create private key used to decrypt the agent deployment data in your Agentuity Cluster.",
			Execute: ExecutionCommand{
				Message: "Creating encryption key...",
				Command: "sh",
				Arguments: []string{
					"-c",
					"echo '{ENCRYPTION_PRIVATE_KEY}' | base64 -d | aws secretsmanager create-secret --name '{AWS_SECRET_NAME}' --description 'Agentuity Cluster Private Key' --secret-binary fileb://-",
				},
				Success:  "Secret created",
				Validate: Validation("{AWS_SECRET_NAME}"),
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking secret...",
				Command: "aws",
				Arguments: []string{
					"secretsmanager",
					"describe-secret",
					"--secret-id",
					"{AWS_SECRET_NAME}",
				},
				Validate: Validation("{AWS_SECRET_NAME}"),
			},
		},
		{
			Title:       "Get Default VPC",
			Description: "Find the default VPC to use for the cluster node.",
			Execute: ExecutionCommand{
				Message: "Finding default VPC...",
				Command: "aws",
				Arguments: []string{
					"ec2",
					"describe-vpcs",
					"--filters",
					"Name=isDefault,Values=true",
					"--query",
					"Vpcs[0].VpcId",
					"--output",
					"text",
				},
				Success: "Found default VPC",
			},
		},
		{
			Title:       "Get Default Subnet",
			Description: "Find a default subnet in the default VPC.",
			Execute: ExecutionCommand{
				Message: "Finding default subnet...",
				Command: "sh",
				Arguments: []string{
					"-c",
					strings.Join([]string{
						"VPC_ID=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)",
						"&&",
						"aws ec2 describe-subnets --filters Name=vpc-id,Values=$VPC_ID Name=default-for-az,Values=true --query 'Subnets[0].SubnetId' --output text",
					}, " "),
				},
				Success: "Found default subnet",
			},
		},
		{
			Title:       "Create Security Group",
			Description: "Create a security group for the Agentuity cluster with necessary ports.",
			Execute: ExecutionCommand{
				Message: "Creating security group...",
				Command: "sh",
				Arguments: []string{
					"-c",
					strings.Join([]string{
						"VPC_ID=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)",
						"&&",
						"aws ec2 create-security-group --group-name {AWS_ROLE_NAME}-sg --description 'Agentuity Cluster Security Group' --vpc-id $VPC_ID --query 'GroupId' --output text",
					}, " "),
				},
				Success: "Security group created",
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking security group...",
				Command: "aws",
				Arguments: []string{
					"ec2",
					"describe-security-groups",
					"--filters",
					"Name=group-name,Values={AWS_ROLE_NAME}-sg",
					"--query",
					"SecurityGroups[0].GroupId",
					"--output",
					"text",
				},
				Validate: Validation("sg-"),
			},
		},
		{
			Title:       "Configure Security Group Rules",
			Description: "Allow SSH and HTTPS traffic for the cluster.",
			Execute: ExecutionCommand{
				Message: "Configuring security group rules...",
				Command: "sh",
				Arguments: []string{
					"-c",
					strings.Join([]string{
						"SG_ID=$(aws ec2 describe-security-groups --filters Name=group-name,Values={AWS_ROLE_NAME}-sg --query 'SecurityGroups[0].GroupId' --output text)",
						"&&",
						"aws ec2 authorize-security-group-ingress --group-id $SG_ID --protocol tcp --port 22 --cidr 0.0.0.0/0 2>/dev/null || true",
						"&&",
						"aws ec2 authorize-security-group-ingress --group-id $SG_ID --protocol tcp --port 443 --cidr 0.0.0.0/0 2>/dev/null || true",
					}, " "),
				},
				Success: "Security group configured",
			},
			SkipIf: &ExecutionCommand{
				Message: "Checking security group rules...",
				Command: "sh",
				Arguments: []string{
					"-c",
					strings.Join([]string{
						"SG_ID=$(aws ec2 describe-security-groups --filters Name=group-name,Values={AWS_ROLE_NAME}-sg --query 'SecurityGroups[0].GroupId' --output text)",
						"&&",
						"aws ec2 describe-security-groups --group-ids $SG_ID --query 'SecurityGroups[0].IpPermissions[?FromPort==\"22\"]' --output text",
					}, " "),
				},
				Validate: Validation("22"),
			},
		},
		{
			Title:       "Get Latest Amazon Linux AMI",
			Description: "Find the latest Amazon Linux 2023 AMI for the region.",
			Execute: ExecutionCommand{
				Message: "Finding latest AMI...",
				Command: "aws",
				Arguments: []string{
					"ec2",
					"describe-images",
					"--owners",
					"amazon",
					"--filters",
					"Name=name,Values=al2023-ami-*-x86_64",
					"Name=state,Values=available",
					"--query",
					"Images | sort_by(@, &CreationDate) | [-1].ImageId",
					"--output",
					"text",
				},
				Success: "Found latest AMI",
			},
		},
		{
			Title:       "Create the Cluster Node",
			Description: "Create a new cluster node instance and launch it.",
			Execute: ExecutionCommand{
				Message: "Creating node...",
				Command: "sh",
				Arguments: []string{
					"-c",
					strings.Join([]string{
						"AMI_ID=$(aws ec2 describe-images --owners amazon --filters 'Name=name,Values=al2023-ami-*-x86_64' 'Name=state,Values=available' --query 'Images | sort_by(@, &CreationDate) | [-1].ImageId' --output text)",
						"&&",
						"SUBNET_ID=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text | xargs -I {} aws ec2 describe-subnets --filters Name=vpc-id,Values={} Name=default-for-az,Values=true --query 'Subnets[0].SubnetId' --output text)",
						"&&",
						"SG_ID=$(aws ec2 describe-security-groups --filters Name=group-name,Values={AWS_ROLE_NAME}-sg --query 'SecurityGroups[0].GroupId' --output text)",
						"&&",
						"aws ec2 run-instances --image-id $AMI_ID --count 1 --instance-type t3.medium --security-group-ids $SG_ID --subnet-id $SUBNET_ID --iam-instance-profile Name={AWS_ROLE_NAME} --user-data '{CLUSTER_TOKEN}' --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value={AWS_INSTANCE_NAME}},{Key=AgentuityCluster,Value={CLUSTER_ID}}]'",
					}, " "),
				},
				Validate: Validation("{AWS_INSTANCE_NAME}"),
				Success:  "Node created",
			},
		},
	}

	for i, s := range spec {
		// Replace variables in Execute arguments
		newArgs := []string{}
		for _, arg := range s.Execute.Arguments {
			for key, val := range envs {
				arg = strings.ReplaceAll(arg, "{"+key+"}", fmt.Sprint(val))
			}
			newArgs = append(newArgs, arg)
		}
		if s.Title == "Create encryption key and store in AWS Secrets Manager" {
			fmt.Printf("DEBUG SECRET: Original args: %v\n", s.Execute.Arguments)
			fmt.Printf("DEBUG SECRET: New args: %v\n", newArgs)
		}
		spec[i].Execute.Arguments = newArgs

		// Replace variables in Execute Validate
		if s.Execute.Validate != "" {
			validate := string(s.Execute.Validate)
			for key, val := range envs {
				validate = strings.ReplaceAll(validate, "{"+key+"}", fmt.Sprint(val))
			}
			spec[i].Execute.Validate = Validation(validate)
		}

		// Replace variables in SkipIf arguments and validation
		if s.SkipIf != nil {
			skipArgs := []string{}
			for _, arg := range s.SkipIf.Arguments {
				for key, val := range envs {
					arg = strings.ReplaceAll(arg, "{"+key+"}", fmt.Sprint(val))
				}
				skipArgs = append(skipArgs, arg)
			}
			spec[i].SkipIf.Arguments = skipArgs

			if s.SkipIf.Validate != "" {
				validate := string(s.SkipIf.Validate)
				for key, val := range envs {
					validate = strings.ReplaceAll(validate, "{"+key+"}", fmt.Sprint(val))
				}
				spec[i].SkipIf.Validate = Validation(validate)
			}
		}
	}
	return spec
}
