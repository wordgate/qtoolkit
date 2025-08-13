package aliyun

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/spf13/viper"
)

func CmsClient(configPath string) *cms.Client {
	config := viper.GetStringMapString(configPath)

	region := config["region"]
	access_key := config["access_key"]
	secret := config["secret"]

	client, err := cms.NewClientWithAccessKey(region, access_key, secret)

	if err != nil {
		panic("Error creating CloudMonitor client:" + err.Error())
	}
	return client
}
