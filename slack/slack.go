package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/viper"
)

func Alert(text string) error {
	return Send("alert", text)
}

func VerifyCode(text string) error {
	return Send("verify_code", text)
}

func Send(channel string, text string) error {

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
