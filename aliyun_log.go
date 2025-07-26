package mods

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	slshook "github.com/innopals/sls-logrus-hook"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// https://sls.console.aliyun.com/lognext/profile
// aliyun.log.url: alconnect.cn_hongkong.log.aliyuncs.com
// aliyun.log.logstore: wgcenter
// aliyun.log.access_key:
// aliyun.log.access_secret:
// aliyun.log.interval_ms: 3000
// aliyun.access_key: YOUR_ALIYUN_ACCESS_KEY
// aliyun.access_secret: YOUR_ALIYUN_ACCESS_SECRET
func AliyunLog(topic string) *logrus.Logger {
	url := viper.GetString("aliyun.log.url")
	accessKey := viper.GetString("aliyun.log.access_key")
	accessSecret := viper.GetString("aliyun.log.access_secret")
	logstore := viper.GetString("aliyun.log.logstore")
	if accessKey == "" || accessSecret == "" {
		accessKey = viper.GetString("aliyun.access_key")
		accessSecret = viper.GetString("aliyun.access_secret")
	}

	hook, err := slshook.NewSlsLogrusHook(url, accessKey, accessSecret, logstore, topic)
	if err != nil {
		panic(err)
	}
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigc
		fmt.Println("flush log in max=10s")
		hook.Flush(10 * time.Second)
		fmt.Println("flush log done")
		os.Exit(1)
	}()

	log := logrus.New()
	log.AddHook(hook)
	//log.SetFormatter(&slshook.NoopFormatter{})
	//log.SetOutput(io.Discard)
	log.SetOutput(os.Stdout)
	levelS := viper.GetString("log_level")
	level, err := logrus.ParseLevel(levelS)
	if err != nil {
		panic(fmt.Sprintf("parse log level failed with error: %v", err))
	}
	log.SetLevel(level)
	return log
}
