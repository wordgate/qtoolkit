package util

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func IsDev() bool {
	return viper.GetBool("is_dev")
}

func IsTest() bool {
	return viper.GetBool("is_test")
}

func SetConfigFile(file string) {
	viper.SetConfigFile(file)
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Sprintf("Fatal error config file: %v \n", err))
	}
	if !IsDev() {
		gin.SetMode(gin.ReleaseMode)
	}
	// Log and DB are now lazy loaded - no need to init here
}
