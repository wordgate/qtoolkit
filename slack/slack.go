package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/viper"
)

func SlackAlert(text string) error {
	return SlackSend("alert", text)
}

func SlackVerifyCode(text string) error {
	return SlackSend("verify_code", text)
}

func SlackSend(channel string, text string) error {

	url := viper.GetString(fmt.Sprintf("slack.%s", channel))
	data := map[string]string{
		"text": text,
	}
	body, _ := json.Marshal(data)

	_, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	return nil
}
