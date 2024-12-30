package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/route53"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiConfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// Helper func to extract the first two octets of a VPC CIDR
// to help in creating subnets.
func extractFirstTwoOctets(cidr string) (string, error) {
	// Split the CIDR string at the slash (/) to separate the IP and the prefix
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid CIDR format: %s", cidr)
	}

	// Split the IP part into octets
	octets := strings.Split(parts[0], ".")
	if len(octets) < 2 {
		return "", fmt.Errorf("invalid IP address format: %s", parts[0])
	}

	// Combine the first two octets
	firstTwoOctets := fmt.Sprintf("%s.%s", octets[0], octets[1])
	return firstTwoOctets, nil
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		pulumiCfg := pulumiConfig.New(ctx, "")
		// Path to the Go Lambda project
		lambdaDir := "./lambda"

		// Run `make` to build and package the Lambda function
		makeCmd := exec.Command("make", "all")
		makeCmd.Dir = lambdaDir
		makeCmd.Stdout = os.Stdout
		makeCmd.Stderr = os.Stderr
		if err := makeCmd.Run(); err != nil {
			return fmt.Errorf("failed to build and package Lambda function: %w", err)
		}
		/********************User Parameters********************/
		allowedCidrRangesList := pulumiCfg.Require("allowedCidrRanges")
		var allowedCidrRanges []string
		// Parse the JSON array into a Go slice to assign to the security group
		if err := json.Unmarshal([]byte(allowedCidrRangesList), &allowedCidrRanges); err != nil {
			return err
		}
		instanceType := pulumiCfg.Require("instanceType")
		amiSsm := pulumiCfg.Require("amiSsm")
		tagKey := pulumiCfg.Require("tagKey")
		tagValue := pulumiCfg.Require("tagValue")
		route53HostName := pulumiCfg.Require("route53HostName")
		route53ZoneId := pulumiCfg.Require("route53ZoneId")
		vpcCidr := pulumiCfg.Require("vpcCidr")
		keypair := pulumiCfg.Require("keypair")
		firstTwoOctets, err := extractFirstTwoOctets(vpcCidr)
		if err != nil {
			fmt.Println("Error:", err)
		}
		/********************User Parameters********************/
		// Load the AWS configuration using the AWS SDK
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			return fmt.Errorf("unable to load SDK config, %w", err)
		}

		// Create an STS client to get the account ID
		stsClient := sts.NewFromConfig(cfg)
		identity, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
		if err != nil {
			return fmt.Errorf("unable to get caller identity, %w", err)
		}

		// Extract the region and account ID
		region := cfg.Region
		accountID := *identity.Account

		// Create an SSM client to retrieve the AMI ID
		ssmClient := ssm.NewFromConfig(cfg)
		ssmOutput, err := ssmClient.GetParameter(context.TODO(), &ssm.GetParameterInput{
			Name: aws.String(amiSsm),
		})
		if err != nil {
			return fmt.Errorf("unable to retrieve AMI ID from SSM, %w", err)
		}
		amiID := *ssmOutput.Parameter.Value

		// Load user data script
		userData, err := os.ReadFile("scripts/userdata.sh")
		if err != nil {
			return err
		}
		vpc, err := ec2.NewVpc(ctx, "minecraftVpc", &ec2.VpcArgs{
			CidrBlock:          pulumi.String(vpcCidr),
			EnableDnsSupport:   pulumi.Bool(true),
			EnableDnsHostnames: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("minecraftVpc"),
				tagKey: pulumi.String(tagValue),
			},
		})
		if err != nil {
			return err
		}

		// Create subnets
		subnetIds := []pulumi.StringOutput{}
		azs := []string{fmt.Sprintf("%sa", region), fmt.Sprintf("%sb", region), fmt.Sprintf("%sc", region)}
		for i, az := range azs {
			subnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("publicSubnet-%s", az), &ec2.SubnetArgs{
				VpcId:               vpc.ID(),
				CidrBlock:           pulumi.String(fmt.Sprintf("%s.%d.0/24", firstTwoOctets, i)),
				AvailabilityZone:    pulumi.String(az),
				MapPublicIpOnLaunch: pulumi.Bool(true),
				Tags: pulumi.StringMap{
					"Name": pulumi.String(fmt.Sprintf("publicSubnet-%s", az)),
					tagKey: pulumi.String(tagValue),
				},
			})
			if err != nil {
				return err
			}
			subnetIds = append(subnetIds, subnet.ID().ToStringOutput())
		}

		// Create Internet Gateway
		igw, err := ec2.NewInternetGateway(ctx, "internetGateway", &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("internetGateway"),
				tagKey: pulumi.String(tagValue),
			},
		})
		if err != nil {
			return err
		}

		// Create Route Table
		routeTable, err := ec2.NewRouteTable(ctx, "routeTable", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: igw.ID(),
				},
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String("routeTable"),
				tagKey: pulumi.String(tagValue),
			},
		})
		if err != nil {
			return err
		}

		// Associate Route Table with Subnets
		for i, subnetId := range subnetIds {
			_, err := ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("routeTableAssociation-%d", i), &ec2.RouteTableAssociationArgs{
				SubnetId:     subnetId,
				RouteTableId: routeTable.ID(),
			})
			if err != nil {
				return err
			}
		}

		// Create a slice of ingress rule arguments
		var ingressRules ec2.SecurityGroupIngressArray
		for _, cidr := range allowedCidrRanges {
			ingressRules = append(ingressRules, ec2.SecurityGroupIngressArgs{
				FromPort:   pulumi.Int(22),
				ToPort:     pulumi.Int(22),
				Protocol:   pulumi.String("tcp"),
				CidrBlocks: pulumi.StringArray{pulumi.String(cidr)},
			})
			ingressRules = append(ingressRules, ec2.SecurityGroupIngressArgs{
				FromPort:   pulumi.Int(25565),
				ToPort:     pulumi.Int(25565),
				Protocol:   pulumi.String("tcp"),
				CidrBlocks: pulumi.StringArray{pulumi.String(cidr)},
			})
		}

		// Create a security group to allow TCP access to ports 80 and 25565
		secGroup, err := ec2.NewSecurityGroup(ctx, "instance-sg", &ec2.SecurityGroupArgs{
			Description: pulumi.String("Allow HTTP and custom port access"),
			VpcId:       vpc.ID(),
			Ingress:     ingressRules,
			Egress: ec2.SecurityGroupEgressArray{
				ec2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
		})
		if err != nil {
			return err
		}

		// IAM Role for EC2 instance
		instanceRole, err := iam.NewRole(ctx, "ec2-instance-role", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {
						"Service": "ec2.amazonaws.com"
					},
					"Action": "sts:AssumeRole"
				}]
			}`),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("ec2-instance-role"),
			},
		})
		if err != nil {
			return err
		}

		_, err = iam.NewRolePolicyAttachment(ctx, "ec2-instance-ssm-policy-attachment", &iam.RolePolicyAttachmentArgs{
			Role:      instanceRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"),
		})
		if err != nil {
			return err
		}

		_, err = iam.NewRolePolicyAttachment(ctx, "ec2-instance-cw-policy-attachment", &iam.RolePolicyAttachmentArgs{
			Role:      instanceRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"),
		})
		if err != nil {
			return err
		}

		// Create an IAM Instance Profile for the EC2 instance
		instanceProfile, err := iam.NewInstanceProfile(ctx, "ec2InstanceProfile", &iam.InstanceProfileArgs{
			Role: instanceRole.Name,
		})
		if err != nil {
			return err
		}

		// Launch EC2 instance
		instance, err := ec2.NewInstance(ctx, "minecraft-blog-instance", &ec2.InstanceArgs{
			InstanceType:        pulumi.String(instanceType),
			VpcSecurityGroupIds: pulumi.StringArray{secGroup.ID()},
			Ami:                 pulumi.String(amiID),
			SubnetId:            subnetIds[0],
			KeyName:             pulumi.String(keypair),
			UserData:            pulumi.String(string(userData)),
			IamInstanceProfile:  instanceProfile.Name,
			Tags:                pulumi.StringMap{tagKey: pulumi.String(tagValue)},
		} /*, pulumi.Protect(true)*/)
		if err != nil {
			return err
		}

		// Allocate Elastic IP and associate it with the EC2 instance
		eip, err := ec2.NewEip(ctx, "instance-ip", &ec2.EipArgs{
			Instance: instance.ID(),
		})
		if err != nil {
			return err
		}

		// Create Route53 A Record
		route53Record, err := route53.NewRecord(ctx, "minecraftARecord", &route53.RecordArgs{
			Name:    pulumi.String(route53HostName),
			ZoneId:  pulumi.String(route53ZoneId),
			Type:    pulumi.String("A"),
			Ttl:     pulumi.Int(300),
			Records: pulumi.StringArray{eip.PublicIp},
		})
		if err != nil {
			return err
		}

		// IAM Role for Lambda
		role, err := iam.NewRole(ctx, "lambda-exec-role", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {
						"Service": "lambda.amazonaws.com"
					},
					"Action": "sts:AssumeRole"
				}]
			}`),
		})
		if err != nil {
			return err
		}

		// Create the policy document with the substituted instance ID
		instance.ID().ApplyT(func(instanceID string) (string, error) {
			policy := fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Action": [
						"ec2:StartInstances",
						"ec2:StopInstances"
					],
					"Resource": "arn:aws:ec2:%s:%s:instance/%s"
				}]
			}`, region, accountID, instanceID)
			return policy, nil
		}).(pulumi.StringOutput).ToStringOutput().ApplyT(func(policy string) (interface{}, error) {
			_, err := iam.NewRolePolicy(ctx, "lambdaPolicy", &iam.RolePolicyArgs{
				Role:   role.ID(),
				Policy: pulumi.String(policy),
			})
			return nil, err
		})

		_, err = iam.NewRolePolicyAttachment(ctx, "lambda-policy-attach", &iam.RolePolicyAttachmentArgs{
			Role:      role.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		})
		if err != nil {
			return err
		}

		// Create the Lambda function
		lambdaFunc, err := lambda.NewFunction(ctx, "minecraft-start-stop", &lambda.FunctionArgs{
			Role:    role.Arn,
			Runtime: pulumi.String("provided.al2023"),
			Handler: pulumi.String("main"),
			Code:    pulumi.NewFileArchive("lambda/main.zip"),
			Architectures: pulumi.StringArray{
				pulumi.String("arm64"),
			},
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"INSTANCE_ID": instance.ID(),
				},
			},
		})
		if err != nil {
			return err
		}

		// Execute lambda binary clean-up only after the Lambda function is created.
		_ = lambdaFunc.Arn.ApplyT(func(arn string) (string, error) {
			fmt.Println("All resources created, running post-deployment `make` task...")
			postMakeCmd := exec.Command("make", "cleanall")
			postMakeCmd.Dir = lambdaDir
			postMakeCmd.Stdout = os.Stdout
			postMakeCmd.Stderr = os.Stderr
			if err := postMakeCmd.Run(); err != nil {
				return "", fmt.Errorf("failed to run post-deployment task: %w", err)
			}
			return "", nil
		}).(pulumi.StringOutput)

		// Export the EC2 instance ID
		ctx.Export("Server Name", route53Record.Fqdn)

		return nil
	})
}
