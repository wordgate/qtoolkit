package mods

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type Ec2Type string

const (
	Ec2Nano    Ec2Type = "t3.nano"
	Ec2Micro   Ec2Type = "t3.micro"
	Ec2Small   Ec2Type = "t3.small"
	Ec2Medium  Ec2Type = "t3.medium"
	Ec2Large   Ec2Type = "t3.large"
	Ec2XLarge  Ec2Type = "t3.xlarge"
	Ec22XLarge Ec2Type = "t3.2xlarge"
)

const Ec2ImageUbuntu20 = "ami-038d76c4d28805c09"

func Ec2Create(region string, typ Ec2Type, sysImage string) (string, error) {
	sess, err := awsSession(region)
	if err != nil {
		return "", err
	}

	svc := ec2.New(sess)

	input := &ec2.RunInstancesInput{
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/xvda"),
				Ebs: &ec2.EbsBlockDevice{
					VolumeSize: aws.Int64(20),
				},
			},
		},
		ImageId:      aws.String(sysImage), // ubuntu20.04
		InstanceType: aws.String(string(typ)),
		MaxCount:     aws.Int64(1),
		MinCount:     aws.Int64(1),
	}

	result, err := svc.RunInstances(input)
	if err != nil {
		fmt.Println("Error creating instance:", err)
		return "", err
	}
	return *result.Instances[0].InstanceId, nil
}

func Ec2Drop(region string, instanceID string) error {
	sess, err := awsSession(region)

	if err != nil {
		fmt.Println("Error drop instance:", err)
		return err
	}

	svc := ec2.New(sess)

	_, err = svc.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	})
	return err
}

func Ec2ReleaseIp(region, instanceID string) error {
	sess, err := awsSession(region)
	if err != nil {
		fmt.Println("Error release ip:", err)
		return err
	}

	svc := ec2.New(sess)

	// Get the public IP address associated with the EC2 instance
	result, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	})
	if err != nil {
		fmt.Println("Error describing EC2 instance:", err)
		return err
	}
	var ipAddress string
	for _, instance := range result.Reservations[0].Instances {
		for _, network := range instance.NetworkInterfaces {
			ipAddress = *network.Association.PublicIp
		}
	}

	// Dissociate the Elastic IP address from the EC2 instance
	_, err = svc.DisassociateAddress(&ec2.DisassociateAddressInput{
		PublicIp: aws.String(ipAddress),
	})
	if err != nil {
		fmt.Println("Error dissociating Elastic IP address:", err)
		return err
	}

	// Release the Elastic IP address
	_, err = svc.ReleaseAddress(&ec2.ReleaseAddressInput{
		PublicIp: aws.String(ipAddress),
	})
	if err != nil {
		fmt.Println("Error releasing Elastic IP address:", err)
		return err
	}

	fmt.Println("Elastic IP address", ipAddress, "dissociated from EC2 instance", instanceID, "and released")
	return nil
}

func Ec2AllocIp(region, instanceID string) (string, error) {
	sess, err := awsSession(region)
	if err != nil {
		fmt.Println("Error creating session:", err)
		return "", err
	}

	svc := ec2.New(sess)

	// Allocate a new Elastic IP address
	result, err := svc.AllocateAddress(&ec2.AllocateAddressInput{
		Domain: aws.String("vpc"),
	})
	if err != nil {
		fmt.Println("Error allocating Elastic IP address:", err)
		return "", err
	}
	ipAddress := *result.PublicIp

	// Associate the Elastic IP address with the EC2 instance
	_, err = svc.AssociateAddress(&ec2.AssociateAddressInput{
		InstanceId: aws.String(instanceID),
		PublicIp:   aws.String(ipAddress),
	})
	if err != nil {
		fmt.Println("Error associating Elastic IP address:", err)
		return "", err
	}
	fmt.Println("Elastic IP address", ipAddress, "assigned to EC2 instance", instanceID)
	return ipAddress, nil
}

func Ec2ExecCmds(region, instanceID string, commands ...string) error {
	sess, err := awsSession(region)
	if err != nil {
		fmt.Println("Error creating session:", err)
		return err
	}

	ssmSvc := ssm.New(sess)
	cmds := []*string{}
	for _, cmd := range commands {
		cmds = append(cmds, aws.String(cmd))
	}

	// Define the parameters for the change-password document
	params := &ssm.SendCommandInput{
		DocumentName: aws.String("AWS-RunShellScript"),
		InstanceIds: []*string{
			aws.String(instanceID),
		},
		Parameters: map[string][]*string{
			"commands": cmds,
		},
	}

	// Call the change-password document
	_, err = ssmSvc.SendCommand(params)
	return err
}
