package qtoolkit

import (
	"testing"

	"github.com/spf13/viper"
)

func TestExchangeApiGet(t *testing.T) {

	viper.Set("exchange_rate.api_key", "YOUR_EXCHANGE_RATE_API_KEY")

	rates, err := ExchangeApiGet("usd")
	if err != nil {
		t.Errorf("exchange api , get failed:%v", err)
	} else if len(rates) == 0 {
		t.Errorf("exchange api , get failed: no rates got")
	}

}
