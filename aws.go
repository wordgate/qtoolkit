package qtoolkit

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/spf13/viper"
)

func awsSession(region string) (*session.Session, error) {
	awsAccessKey := viper.GetString("aws.access_key")
	awsSecret := viper.GetString("aws.secret")

	return session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(awsAccessKey, awsSecret, ""),
		},
		SharedConfigState: session.SharedConfigEnable,
	})
}
