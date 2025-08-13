package main

import (
	"fmt"
	"log"

	"github.com/wordgate/qtoolkit/core"
	"github.com/wordgate/qtoolkit/aws"
	"github.com/wordgate/qtoolkit/slack"
)

func main() {
	// Initialize configuration
	core.SetConfigFile("example_config.yml")
	
	// Check if AWS module is enabled
	awsConfig := core.GetModuleConfig("aws")
	if awsConfig.Enabled {
		fmt.Println("AWS module is enabled")
		// AWS functions are available through aws package
		_ = aws.EC2Info{}
	}
	
	// Check if Slack module is enabled
	slackConfig := core.GetModuleConfig("slack")
	if slackConfig.Enabled {
		fmt.Println("Slack module is enabled")
		// Slack functions are available through slack package
		err := slack.SendSlackMessage("Hello from modular qtoolkit!")
		if err != nil {
			log.Printf("Failed to send Slack message: %v", err)
		}
	}
	
	// Modules that are not enabled won't be imported/compiled
	aliyunConfig := core.GetModuleConfig("aliyun")
	if !aliyunConfig.Enabled {
		fmt.Println("Aliyun module is disabled - no compilation overhead")
	}
}