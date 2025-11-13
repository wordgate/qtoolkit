package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/viper"
)

func ShortUrl(url string, password string, days int) (string, error) {
	return dwz_lc(url, password, days)
}

func dwz_lc(url string, password string, days int) (string, error) {
	// https://www.dwz.lc/user/tools/api
	data := map[string]interface{}{
		"url": url,
	}
	if days > 0 {
		data["expiry"] = time.Now().AddDate(0, 0, days+1).Format("2006-01-02")
	}
	if password != "" {
		data["password"] = password
	}
	body, _ := json.Marshal(data)
	token := viper.GetString("dwz_lc.token")

	r, err := http.NewRequest("POST", "https://www.dwz.lc/api/url/add", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Authorization", fmt.Sprintf("Token %s", token))

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	res_body, _ := io.ReadAll(resp.Body)
	res_data := struct {
		Error int    `json:"error"`
		Short string `json:"short"`
		Msg   string `json:"msg"`
	}{}

	err = json.Unmarshal(res_body, &res_data)
	if err != nil {
		return "", err
	}

	if res_data.Error == 0 {
		return res_data.Short, nil
	}
	return "", errors.New(res_data.Msg)
}
