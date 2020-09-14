package main

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/zouyx/agollo/v3/component/log"
	"gopkg.in/alecthomas/kingpin.v2"
	"xorm.io/xorm"
)

var (
	DATAFILE   = "data.db"
	ServerAddr = kingpin.Flag("addr", "server addr").Default(":8080").String()
	ConfigDir  = kingpin.Flag("config-dir", "config dir contain channel config toml").Default("./conf").Short('c').String()
)

type (
	Config struct {
		Channel    []*ChannelConf
		channelMap map[string]*ChannelConf
	}
)

func (conf *Config) Check(repository *Repository) error {
	conf.channelMap = map[string]*ChannelConf{}
	for _, c := range conf.Channel {
		err := c.CheckConf(repository)
		if err != nil {
			return err
		}
		conf.channelMap[c.Desc.Title] = c
	}
	return nil
}

func (conf *Config) LoadConfig(dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Error(err)
		return
	}
	conf.Channel = []*ChannelConf{}
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".toml") {
			cconf := ChannelConf{}
			filePath := path.Join(dir, f.Name())
			_, err = toml.DecodeFile(filePath, &cconf)
			if err != nil {
				logrus.Errorf("read config fail for %s:%v", filePath, err)
				continue
			}
			logrus.Infof("load config file: %s", filePath)
			conf.Channel = append(conf.Channel, &cconf)
		}
	}
}

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
	})
}

func main() {
	kingpin.Parse()
	engine, err := xorm.NewEngine("sqlite3", DATAFILE)
	if err != nil {
		logrus.Fatal(err)
	}
	err = engine.CreateTables(&Item{})
	if err != nil {
		logrus.Fatal(err)
	}
	repository := newRepository(engine)
	CONFIG := &Config{}
	CONFIG.LoadConfig(*ConfigDir)
	if err = CONFIG.Check(repository); err != nil {
		logrus.Fatal(err)
	}
	go func() {
		for {
			for _, channel := range CONFIG.Channel {
				if err := channel.Update(); err != nil {
					logrus.Errorf("update item for %s:%v", channel.Desc.Title, err)
				}
			}
			<-time.After(time.Hour)
		}
	}()
	route := gin.Default()
	route.GET("/rss/:channel", func(ctx *gin.Context) {
		channelName := ctx.Param("channel")
		channel, ok := CONFIG.channelMap[channelName]
		if !ok {
			_ = ctx.AbortWithError(404, fmt.Errorf("channelName %s not found", channelName))
			return
		}
		body, err := channel.ToRss()
		if err != nil {
			_ = ctx.AbortWithError(500, err)
			return
		}
		_, _ = ctx.Writer.Write(body)
	})
	route.GET("/rss", func(ctx *gin.Context) {
		fileName := []string{}
		for k := range CONFIG.channelMap {
			fileName = append(fileName, k)
		}
		ctx.JSON(200, gin.H{"rss": fileName})
	})
	if err = route.Run(*ServerAddr); err != nil {
		logrus.Fatal(err)
	}
}
