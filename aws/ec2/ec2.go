package ec2

import (
	"context"
	"fmt"
	"sync"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/spf13/viper"
)

// Config represents EC2 configuration
type Config struct {
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	UseIMDS   bool   `yaml:"use_imds" json:"use_imds"`
	Region    string `yaml:"region" json:"region"`
}

var (
	globalConfig *Config
	globalClient *ec2.Client
	clientOnce   sync.Once
	initErr      error
	configMux    sync.RWMutex
)

// loadConfigFromViper loads EC2 configuration from viper
// Configuration path priority (cascading fallback):
// 1. aws.ec2 - EC2 service config
// 2. aws - Global AWS config
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	// Load EC2-specific config
	cfg.Region = viper.GetString("aws.ec2.region")
	cfg.AccessKey = viper.GetString("aws.ec2.access_key")
	cfg.SecretKey = viper.GetString("aws.ec2.secret_key")
	cfg.UseIMDS = viper.GetBool("aws.ec2.use_imds")

	// Fall back to global AWS config for missing credentials/region
	if cfg.Region == "" {
		cfg.Region = viper.GetString("aws.region")
	}
	if cfg.AccessKey == "" {
		cfg.AccessKey = viper.GetString("aws.access_key")
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = viper.GetString("aws.secret_key")
	}
	if !viper.IsSet("aws.ec2.use_imds") && viper.IsSet("aws.use_imds") {
		cfg.UseIMDS = viper.GetBool("aws.use_imds")
	}

	// Validate required fields
	if cfg.Region == "" {
		return nil, fmt.Errorf("ec2 region not configured (check aws.region or aws.ec2.region)")
	}

	return cfg, nil
}

// SetConfig sets the EC2 configuration for lazy loading (deprecated)
// Use viper configuration instead
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	globalConfig = cfg
}

// GetConfig returns the current EC2 configuration
func GetConfig() *Config {
	configMux.RLock()
	defer configMux.RUnlock()
	return globalConfig
}

// initialize performs the actual EC2 client initialization
func initialize() {
	// Try to load from viper first
	cfg, err := loadConfigFromViper()
	if err != nil {
		// Fall back to SetConfig if viper config not available
		configMux.RLock()
		cfg = globalConfig
		configMux.RUnlock()

		if cfg == nil {
			initErr = fmt.Errorf("EC2 config not available: %v", err)
			return
		}
	} else {
		// Store loaded config
		configMux.Lock()
		globalConfig = cfg
		configMux.Unlock()
	}

	if cfg.Region == "" {
		initErr = fmt.Errorf("EC2 region is required")
		return
	}

	awsCfg, err := loadConfig(cfg.Region, cfg)
	if err != nil {
		initErr = fmt.Errorf("failed to load AWS config: %v", err)
		return
	}

	globalClient = ec2.NewFromConfig(awsCfg)
	initErr = nil
}

// getClient returns the EC2 client with lazy initialization
func getClient() (*ec2.Client, error) {
	clientOnce.Do(initialize)
	if initErr != nil {
		return nil, initErr
	}
	return globalClient, nil
}

// InstanceType represents EC2 instance type
type InstanceType string

const (
	InstanceNano    InstanceType = "t3.nano"
	InstanceMicro   InstanceType = "t3.micro"
	InstanceSmall   InstanceType = "t3.small"
	InstanceMedium  InstanceType = "t3.medium"
	InstanceLarge   InstanceType = "t3.large"
	InstanceXLarge  InstanceType = "t3.xlarge"
	Instance2XLarge InstanceType = "t3.2xlarge"
)

// Common AMI images
const (
	ImageUbuntu20 = "ami-038d76c4d28805c09"
)

// loadConfig loads AWS configuration for EC2
func loadConfig(region string, cfg *Config) (awsv2.Config, error) {
	ctx := context.Background()

	// If UseIMDS is explicitly set to false, use static credentials
	if cfg != nil && !cfg.UseIMDS {
		if cfg.AccessKey != "" && cfg.SecretKey != "" {
			return awsconfig.LoadDefaultConfig(ctx,
				awsconfig.WithRegion(region),
				awsconfig.WithCredentialsProvider(awscredentials.NewStaticCredentialsProvider(
					cfg.AccessKey,
					cfg.SecretKey,
					"",
				)),
			)
		}
		return awsv2.Config{}, fmt.Errorf("UseIMDS is false but AccessKey/SecretKey are not configured")
	}

	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
}

// CreateInstance creates a new EC2 instance
func CreateInstance(cfg *Config, typ InstanceType, sysImage string) (string, error) {
	if cfg == nil || cfg.Region == "" {
		return "", fmt.Errorf("EC2 config not set or region missing")
	}

	awsCfg, err := loadConfig(cfg.Region, cfg)
	if err != nil {
		return "", err
	}

	client := ec2.NewFromConfig(awsCfg)
	ctx := context.Background()

	input := &ec2.RunInstancesInput{
		BlockDeviceMappings: []ec2types.BlockDeviceMapping{
			{
				DeviceName: awsv2.String("/dev/xvda"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeSize: awsv2.Int32(20),
				},
			},
		},
		ImageId:      awsv2.String(sysImage),
		InstanceType: ec2types.InstanceType(typ),
		MaxCount:     awsv2.Int32(1),
		MinCount:     awsv2.Int32(1),
	}

	result, err := client.RunInstances(ctx, input)
	if err != nil {
		return "", fmt.Errorf("error creating instance: %v", err)
	}

	return *result.Instances[0].InstanceId, nil
}

// TerminateInstance terminates an EC2 instance
func TerminateInstance(cfg *Config, instanceID string) error {
	if cfg == nil || cfg.Region == "" {
		return fmt.Errorf("EC2 config not set or region missing")
	}

	awsCfg, err := loadConfig(cfg.Region, cfg)
	if err != nil {
		return err
	}

	client := ec2.NewFromConfig(awsCfg)
	ctx := context.Background()

	_, err = client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("error terminating instance: %v", err)
	}

	return nil
}

// ReleaseIP dissociates and releases an Elastic IP from an EC2 instance
func ReleaseIP(cfg *Config, instanceID string) error {
	if cfg == nil || cfg.Region == "" {
		return fmt.Errorf("EC2 config not set or region missing")
	}

	awsCfg, err := loadConfig(cfg.Region, cfg)
	if err != nil {
		return err
	}

	client := ec2.NewFromConfig(awsCfg)
	ctx := context.Background()

	// Get the public IP address associated with the EC2 instance
	result, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("error describing EC2 instance: %v", err)
	}

	var ipAddress string
	if len(result.Reservations) > 0 && len(result.Reservations[0].Instances) > 0 {
		for _, network := range result.Reservations[0].Instances[0].NetworkInterfaces {
			if network.Association != nil && network.Association.PublicIp != nil {
				ipAddress = *network.Association.PublicIp
				break
			}
		}
	}

	if ipAddress == "" {
		return fmt.Errorf("no public IP found for instance %s", instanceID)
	}

	// Dissociate the Elastic IP address from the EC2 instance
	_, err = client.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{
		PublicIp: awsv2.String(ipAddress),
	})
	if err != nil {
		return fmt.Errorf("error dissociating Elastic IP address: %v", err)
	}

	// Release the Elastic IP address
	_, err = client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{
		PublicIp: awsv2.String(ipAddress),
	})
	if err != nil {
		return fmt.Errorf("error releasing Elastic IP address: %v", err)
	}

	fmt.Printf("Elastic IP address %s dissociated from EC2 instance %s and released\n", ipAddress, instanceID)
	return nil
}

// AllocateIP allocates a new Elastic IP and associates it with an EC2 instance
func AllocateIP(cfg *Config, instanceID string) (string, error) {
	if cfg == nil || cfg.Region == "" {
		return "", fmt.Errorf("EC2 config not set or region missing")
	}

	awsCfg, err := loadConfig(cfg.Region, cfg)
	if err != nil {
		return "", err
	}

	client := ec2.NewFromConfig(awsCfg)
	ctx := context.Background()

	// Allocate a new Elastic IP address
	result, err := client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: ec2types.DomainTypeVpc,
	})
	if err != nil {
		return "", fmt.Errorf("error allocating Elastic IP address: %v", err)
	}
	ipAddress := *result.PublicIp

	// Associate the Elastic IP address with the EC2 instance
	_, err = client.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		InstanceId: awsv2.String(instanceID),
		PublicIp:   awsv2.String(ipAddress),
	})
	if err != nil {
		return "", fmt.Errorf("error associating Elastic IP address: %v", err)
	}

	fmt.Printf("Elastic IP address %s assigned to EC2 instance %s\n", ipAddress, instanceID)
	return ipAddress, nil
}

// ExecuteCommands executes shell commands on an EC2 instance via AWS Systems Manager
func ExecuteCommands(cfg *Config, instanceID string, commands ...string) error {
	if cfg == nil || cfg.Region == "" {
		return fmt.Errorf("EC2 config not set or region missing")
	}

	awsCfg, err := loadConfig(cfg.Region, cfg)
	if err != nil {
		return err
	}

	client := ssm.NewFromConfig(awsCfg)
	ctx := context.Background()

	// Define the parameters for the AWS-RunShellScript document
	params := &ssm.SendCommandInput{
		DocumentName: awsv2.String("AWS-RunShellScript"),
		InstanceIds:  []string{instanceID},
		Parameters: map[string][]string{
			"commands": commands,
		},
	}

	// Execute the command
	_, err = client.SendCommand(ctx, params)
	if err != nil {
		return fmt.Errorf("error executing commands: %v", err)
	}

	return nil
}
