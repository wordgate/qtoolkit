package slack

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

func TestSlackAlert(t *testing.T) {
	viper.Set("slack.alert", "https://hooks.slack.com/services/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")

	err := SlackAlert("test")
	if err != nil {
		t.Errorf("slack alert, get failed:%v", err)
	} else {
		fmt.Printf("Slack alert, don")
	}
}
