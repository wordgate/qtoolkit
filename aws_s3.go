package mods

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type uploadFunc func(objKey string, body io.ReadSeeker) (string, error)

func S3Upload(objKey string, body io.ReadSeeker) (string, error) {
	bucket := viper.GetString("aws.s3.bucket")
	region := viper.GetString("aws.s3.region")
	urlPrefix := strings.TrimRight(viper.GetString("aws.s3.url_prefix"), "/") + "/"
	objKey = strings.TrimLeft(objKey, "/")

	session, err := awsSession(region)
	if err != nil {
		return "", err
	}

	svc := s3.New(session)
	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objKey),
		Body:   body,
	})
	return urlPrefix + objKey, err
}

func S3UploadBytes(objKey string, byts []byte) (string, error) {
	return S3Upload(objKey, bytes.NewReader(byts))
}

func S3HandleImageUpload(
	keyF func(c *gin.Context) string,
	before func(c *gin.Context, file io.ReadSeeker) (io.ReadSeekCloser, error),
	done func(c *gin.Context, url string) error) gin.HandlerFunc {

	return func(c *gin.Context) {
		objKey := keyF(c)

		file, err := c.FormFile("file")
		if err != nil {
			c.AbortWithStatus(400)
			return
		}

		ext := filepath.Ext(file.Filename)
		if !(ext == ".jpg" || ext == ".png" || ext == ".jpeg") {
			c.AbortWithStatus(400)
			return
		}
		f, _ := file.Open()
		var tf io.ReadSeekCloser = f
		if before != nil {
			tf, err = before(c, f)
			if err != nil {
				c.AbortWithStatus(400)
				return
			}
		}
		defer tf.Close()

		url, err := S3Upload(objKey, tf)
		if err != nil {
			c.AbortWithStatus(500)
			return
		}
		err = done(c, url)
		if err != nil {
			c.AbortWithStatus(500)
			return
		}
	}
}
