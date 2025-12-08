package ssm

import (
	"context"
	"fmt"
	"sync"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/spf13/viper"
)

// Config represents SSM configuration
type Config struct {
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	UseIMDS   bool   `yaml:"use_imds" json:"use_imds"`
	Region    string `yaml:"region" json:"region"`
}

// ParameterType represents the type of SSM parameter
type ParameterType string

const (
	ParameterTypeString       ParameterType = "String"
	ParameterTypeStringList   ParameterType = "StringList"
	ParameterTypeSecureString ParameterType = "SecureString"
)

var (
	globalConfig *Config
	globalClient *ssm.Client
	clientOnce   sync.Once
	initErr      error
	configMux    sync.RWMutex
)

// loadConfigFromViper loads SSM configuration from viper
// Configuration path priority (cascading fallback):
// 1. aws.ssm - SSM service config
// 2. aws - Global AWS config
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	// Load SSM-specific config
	cfg.Region = viper.GetString("aws.ssm.region")
	cfg.AccessKey = viper.GetString("aws.ssm.access_key")
	cfg.SecretKey = viper.GetString("aws.ssm.secret_key")
	cfg.UseIMDS = viper.GetBool("aws.ssm.use_imds")

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
	if !viper.IsSet("aws.ssm.use_imds") && viper.IsSet("aws.use_imds") {
		cfg.UseIMDS = viper.GetBool("aws.use_imds")
	}

	// Validate required fields
	if cfg.Region == "" {
		return nil, fmt.Errorf("ssm region not configured (check aws.region or aws.ssm.region)")
	}

	return cfg, nil
}

// initialize performs the actual SSM client initialization
func initialize() {
	cfg, err := loadConfigFromViper()
	if err != nil {
		initErr = fmt.Errorf("failed to load SSM config: %v", err)
		return
	}

	// Store config for later use
	configMux.Lock()
	globalConfig = cfg
	configMux.Unlock()

	ctx := context.Background()
	var awsCfg awsv2.Config

	// If UseIMDS is explicitly set to false, use static credentials
	if !cfg.UseIMDS {
		if cfg.AccessKey != "" && cfg.SecretKey != "" {
			awsCfg, err = config.LoadDefaultConfig(ctx,
				config.WithRegion(cfg.Region),
				config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
					cfg.AccessKey,
					cfg.SecretKey,
					"",
				)),
			)
		} else {
			initErr = fmt.Errorf("UseIMDS is false but AccessKey/SecretKey are not configured")
			return
		}
	} else {
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	}

	if err != nil {
		initErr = fmt.Errorf("failed to load AWS config: %v", err)
		return
	}

	globalClient = ssm.NewFromConfig(awsCfg)
	initErr = nil
}

// getClient returns the SSM client with lazy initialization
func getClient() (*ssm.Client, error) {
	clientOnce.Do(initialize)
	if initErr != nil {
		return nil, initErr
	}
	return globalClient, nil
}

// GetParameter gets a single SSM parameter value (automatically decrypts SecureString)
func GetParameter(name string) (string, error) {
	client, err := getClient()
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	input := &ssm.GetParameterInput{
		Name:           awsv2.String(name),
		WithDecryption: awsv2.Bool(true), // Automatically decrypt SecureString type
	}

	result, err := client.GetParameter(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get SSM parameter %s: %w", name, err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return "", fmt.Errorf("SSM parameter %s is empty", name)
	}

	return *result.Parameter.Value, nil
}

// Parameter represents an SSM parameter with metadata
type Parameter struct {
	Name  string
	Value string
	Type  ParameterType
	ARN   string
}

// GetParameters gets multiple SSM parameters in batch (automatically decrypts)
// Returns a map of parameter names to values for successfully retrieved parameters
// Invalid parameter names are silently ignored (as per AWS SSM behavior)
func GetParameters(names []string) (map[string]string, error) {
	if len(names) == 0 {
		return make(map[string]string), nil
	}

	client, err := getClient()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	input := &ssm.GetParametersInput{
		Names:          names,
		WithDecryption: awsv2.Bool(true),
	}

	result, err := client.GetParameters(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSM parameters: %w", err)
	}

	// Build result map
	params := make(map[string]string, len(result.Parameters))
	for _, param := range result.Parameters {
		if param.Name != nil && param.Value != nil {
			params[*param.Name] = *param.Value
		}
	}

	return params, nil
}

// GetParametersWithMetadata gets multiple SSM parameters with full metadata
func GetParametersWithMetadata(names []string) ([]*Parameter, error) {
	if len(names) == 0 {
		return []*Parameter{}, nil
	}

	client, err := getClient()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	input := &ssm.GetParametersInput{
		Names:          names,
		WithDecryption: awsv2.Bool(true),
	}

	result, err := client.GetParameters(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSM parameters: %w", err)
	}

	// Build result slice
	params := make([]*Parameter, 0, len(result.Parameters))
	for _, param := range result.Parameters {
		if param.Name != nil && param.Value != nil {
			p := &Parameter{
				Name:  *param.Name,
				Value: *param.Value,
				Type:  ParameterType(param.Type),
			}
			if param.ARN != nil {
				p.ARN = *param.ARN
			}
			params = append(params, p)
		}
	}

	return params, nil
}

// PutParameterOptions configures parameter creation/update
type PutParameterOptions struct {
	Type        ParameterType // Parameter type (default: String)
	Description string        // Parameter description (optional)
	Overwrite   bool          // Overwrite existing parameter (default: false)
}

// PutParameter creates or updates an SSM parameter
func PutParameter(name, value string, opts *PutParameterOptions) error {
	client, err := getClient()
	if err != nil {
		return err
	}

	// Default options
	if opts == nil {
		opts = &PutParameterOptions{
			Type:      ParameterTypeString,
			Overwrite: true, // Default to overwrite for convenience
		}
	}

	// Default to String type if not specified
	paramType := opts.Type
	if paramType == "" {
		paramType = ParameterTypeString
	}

	ctx := context.Background()
	input := &ssm.PutParameterInput{
		Name:      awsv2.String(name),
		Value:     awsv2.String(value),
		Type:      types.ParameterType(paramType),
		Overwrite: awsv2.Bool(opts.Overwrite),
	}

	if opts.Description != "" {
		input.Description = awsv2.String(opts.Description)
	}

	_, err = client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put SSM parameter %s: %w", name, err)
	}

	return nil
}

// PutSecureString creates or updates a SecureString parameter (convenience function)
func PutSecureString(name, value, description string) error {
	return PutParameter(name, value, &PutParameterOptions{
		Type:        ParameterTypeSecureString,
		Description: description,
		Overwrite:   true,
	})
}

// DeleteParameter deletes an SSM parameter
func DeleteParameter(name string) error {
	client, err := getClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	input := &ssm.DeleteParameterInput{
		Name: awsv2.String(name),
	}

	_, err = client.DeleteParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete SSM parameter %s: %w", name, err)
	}

	return nil
}

// DeleteParameters deletes multiple SSM parameters in batch
func DeleteParameters(names []string) error {
	if len(names) == 0 {
		return nil
	}

	client, err := getClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	input := &ssm.DeleteParametersInput{
		Names: names,
	}

	result, err := client.DeleteParameters(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete SSM parameters: %w", err)
	}

	// Check for invalid parameters
	if len(result.InvalidParameters) > 0 {
		return fmt.Errorf("failed to delete some parameters: %v", result.InvalidParameters)
	}

	return nil
}

// Reset resets the SSM client and configuration
// This is mainly useful for testing
func Reset() {
	configMux.Lock()
	defer configMux.Unlock()

	globalConfig = nil
	globalClient = nil
	initErr = nil
	clientOnce = sync.Once{}
}
