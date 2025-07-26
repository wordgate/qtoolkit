package qtoolkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/viper"
)

type ListMonkList struct {
	ID int `json:"id"`
}

type ListMonkUserWithoutID struct {
	Email                   string                 `json:"email"`
	Name                    string                 `json:"name"`
	Attribs                 map[string]interface{} `json:"attribs"`
	Lists                   []int                  `json:"lists"`
	Status                  string                 `json:"status"`
	PreconfirmSubscriptions bool                   `json:"preconfirm_subscriptions"`
}

type ListMonkUser struct {
	ListMonkUserWithoutID
	ID    int            `json:"id"`
	UUID  string         `json:"uuid"`
	Lists []ListMonkList `json:"lists"`
}

func ListMonk_Request(method, path string, body interface{}) ([]byte, error) {
	baseUrl := viper.GetString("listmonk.url")
	url := strings.TrimRight(baseUrl, "/") + path
	bt, _ := json.Marshal(body)
	req, err := http.NewRequest(method, url, bytes.NewReader(bt))
	if err != nil {
		fmt.Printf("send request:%s,(%s) err: %v", url, string(bt), err)
		return nil, err
	}
	username := viper.GetString("listmonk.username")
	password := viper.GetString("listmonk.password")

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(username, password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("send request:%s,(%s) err: %v", url, string(bt), err)
		return nil, err
	}

	bts, err := io.ReadAll(res.Body)
	fmt.Printf("send request:%s,(%s) response: %s", url, string(bt), string(bts))
	return bts, err
}

func ListMonk_GetExistUser(email string) (*ListMonkUser, error) {
	path := fmt.Sprintf("/api/subscribers?query=subscribers.email='%s'", email)

	bodyText, err := ListMonk_Request("GET", path, nil)
	if err != nil {
		return nil, err
	}

	if !strings.Contains(string(bodyText), email) {
		return nil, nil
	}
	data := map[string]map[string]interface{}{}
	json.Unmarshal(bodyText, &data)
	b, _ := json.Marshal(data["data"]["results"])
	users := []ListMonkUser{}
	json.Unmarshal(b, &users)
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

func ListMonk_SyncUser(user *ListMonkUserWithoutID) error {
	exists, err := ListMonk_GetExistUser(user.Email)
	if err != nil {
		return err
	}
	if exists != nil {
		for _, listObj := range exists.Lists {
			user.Lists = append(user.Lists, listObj.ID)
		}
		for k, v := range exists.Attribs {
			user.Attribs[k] = v
		}
		user.Status = "enabled"
		_, err = ListMonk_Request("PUT", fmt.Sprintf("/api/subscribers/%d", exists.ID), user)
	} else {
		_, err = ListMonk_Request("POST", "/api/subscribers", user)
	}
	return err
}
