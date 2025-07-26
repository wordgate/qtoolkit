package qtoolkit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/viper"
)

type ExchangeApiResp struct {
	TimeLastUpdateUnix int64              `json:"time_last_update_unix"`
	Result             string             `json:"result"`
	ConversionRates    map[string]float64 `json:"conversion_rates"` // upper case map
}

func ExchangeApiGet(currency string) (map[string]float64, error) {
	apiKey := viper.GetString("exchange_rate.api_key")
	url := fmt.Sprintf("https://v6.exchangerate-api.com/v6/%s/latest/%s", apiKey, strings.ToUpper(currency))

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	ex := &ExchangeApiResp{}
	err = json.Unmarshal(body, ex)
	return ex.ConversionRates, err
}
