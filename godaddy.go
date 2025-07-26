package mods

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/viper"
)

type godaddyResp struct {
	body []byte
}

func (r *godaddyResp) Sync(container interface{}) error {
	return json.Unmarshal(r.body, &container)
}

func godaddyRequest(method string, path string, data interface{}) (*godaddyResp, error) {
	baseUrl := viper.GetString("godaddy.base_url")
	key := viper.GetString("godaddy.key")
	secret := viper.GetString("godaddy.secret")

	var byt []byte
	if data != nil {
		byt, _ = json.Marshal(data)
	}
	buffer := bytes.NewBuffer(byt)

	url := strings.TrimRight(baseUrl, "/") + path

	req, err := http.NewRequest(method, url, buffer)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// https://developer.godaddy.com/doc/endpoint/domains#/v1/recordAdd
	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		return &godaddyResp{body: body}, err
	}
	return nil, fmt.Errorf("request godaddy err with status:%d", resp.StatusCode)
}

func GodaddyDomainAddARecord(domain, name, data string) error {
	path := fmt.Sprintf("/v1/domains/%s/records", domain)
	body := []interface{}{
		map[string]interface{}{
			"data":     data,
			"name":     name,
			"port":     65535,
			"priority": 0,
			"protocol": "string",
			"service":  "string",
			"ttl":      600,
			"type":     "A",
			"weight":   1,
		},
	}
	_, err := godaddyRequest(http.MethodPatch, path, body)
	return err
}

func GodaddyDomainDelARecord(domain, name string) error {
	path := fmt.Sprintf("/v1/domains/%s/records/A/%s", domain, name)
	_, err := godaddyRequest(http.MethodDelete, path, nil)
	return err
}

func GodaddyDomainGetARecord(domain, name string) (string, error) {
	path := fmt.Sprintf("/v1/domains/%s/records/A/%s", domain, name)
	resp, err := godaddyRequest(http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	data := []map[string]interface{}{}
	err = resp.Sync(&data)
	if v, ok := data[0]["data"]; ok {
		return v.(string), nil
	}
	return "", err
}
