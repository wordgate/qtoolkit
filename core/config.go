package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wordgate/qtoolkit/core/log"

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
	topic := filepath.Base(os.Args[0])
	log.InitLogger(topic)
}

// ModuleConfig represents configuration for optional modules
type ModuleConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ConfigFile string `yaml:"config_file,omitempty"`
}

// GetModuleConfig returns module configuration
func GetModuleConfig(moduleName string) ModuleConfig {
	config := ModuleConfig{}
	if viper.IsSet(fmt.Sprintf("modules.%s", moduleName)) {
		viper.UnmarshalKey(fmt.Sprintf("modules.%s", moduleName), &config)
	}
	return config
}
