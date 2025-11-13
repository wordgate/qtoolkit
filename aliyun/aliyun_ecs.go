package aliyun

import (
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/spf13/viper"
)

func Configs() []string {
	configs := []string{}
	for k := range viper.GetStringMap("aliyun") {
		configs = append(configs, k)
	}
	return configs
}

func Regions(config string) ([]string, error) {
	regions := []string{}
	client := ClientGet(config)
	req := ecs.CreateDescribeRegionsRequest()
	regionsResp, err := client.DescribeRegions(req)
	if err != nil {
		return nil, err
	}

	for _, region := range regionsResp.Regions.Region {
		regions = append(regions, region.RegionId)
	}
	return regions, nil
}

func ClientGetDefault() *ecs.Client {
	return ClientGet("default")
}

func ClientGet(config string) *ecs.Client {
	region := viper.GetString(fmt.Sprintf("aliyun.ecs.%s.region", config))
	access_key := viper.GetString(fmt.Sprintf("aliyun.ecs.%s.access_key", config))
	secret := viper.GetString(fmt.Sprintf("aliyun.ecs.%s.secret", config))

	// fmt.Printf("region: %s, access_key: %s, secret: %s\n", region, access_key, secret)

	client, err := ecs.NewClientWithAccessKey(region, access_key, secret)
	if err != nil {
		fmt.Println("Error creating Aliyun client:", err)
		panic("aliyun Ecs config is not correct, config=" + config)
	}
	return client
}

func Metrics(config string, instanceID, metricName, period, startTime, endTime string) (*cms.DescribeMetricLastResponse, error) {
	client := CmsClient(fmt.Sprintf("aliyun.ecs.%s", config))
	req := cms.CreateDescribeMetricLastRequest()
	req.Scheme = "https"
	req.Namespace = "acs_ecs_dashboard"
	req.MetricName = metricName
	req.Dimensions = fmt.Sprintf("[{\"instanceId\":\"%s\"}]", instanceID)
	req.Period = period
	req.StartTime = startTime
	req.EndTime = endTime

	resp, err := client.DescribeMetricLast(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func ListDefault(regions []string) ([]ecs.Instance, error) {
	return List("default", regions)
}

func List(config string, regions []string) ([]ecs.Instance, error) {
	var instances []ecs.Instance

	if len(regions) == 0 {
		var err error
		regions, err = Regions(config)
		if err != nil {
			return nil, err
		}
	}
	client := ClientGet(config)

	for _, region := range regions {
		req := ecs.CreateDescribeInstancesRequest()
		req.Scheme = "https"
		req.RegionId = region

		resp, err := client.DescribeInstances(req)
		if err != nil {
			return nil, err
		}
		instances = append(instances, resp.Instances.Instance...)
	}
	return instances, nil
}

func Create(config string, region string, instanceType string, imageID string) (string, error) {
	req := ecs.CreateRunInstancesRequest()
	req.Scheme = "https"
	req.RegionId = region
	req.InstanceType = instanceType
	req.ImageId = imageID

	client := ClientGet(config)

	resp, err := client.RunInstances(req)
	if err != nil {
		return "", err
	}
	return resp.InstanceIdSets.InstanceIdSet[0], nil
}

func Delete(config string, instanceID string) error {
	req := ecs.CreateDeleteInstanceRequest()
	req.Scheme = "https"
	req.InstanceId = instanceID

	client := ClientGet(config)
	_, err := client.DeleteInstance(req)
	return err
}

func Detail(config string, instanceID string) (*ecs.Instance, error) {
	req := ecs.CreateDescribeInstancesRequest()
	req.Scheme = "https"
	req.InstanceIds = fmt.Sprintf("[\"%s\"]", instanceID)

	resp, err := ClientGet(config).DescribeInstances(req)
	if err != nil {
		return nil, err
	}

	if len(resp.Instances.Instance) == 0 {
		return nil, fmt.Errorf("instance not found")
	}

	return &resp.Instances.Instance[0], nil
}
