package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	common "github.com/zhnxin/common-go"
	"github.com/zouyx/agollo/v3/component/log"
	"gopkg.in/alecthomas/kingpin.v2"
	"xorm.io/xorm"
)

var (
	DATAFILE  = "data.db"
	CONF_DIR  = ".web2rss"
	USER_DIR  string
	BASE_CONF *BaseConfig
	Cmd       = kingpin.Arg("command", "action comand").Required().Enum("start", "stop", "status", "reload")
)

const SOCKET_FILE = ".web2rss.socket"

type (
	Config struct {
		Channel    []*ChannelConf
		channelMap map[string]*ChannelConf
	}
	BaseConfig struct {
		Addr      string
		Token     string
		ConfigDir string
		userDir   string
		Period    int
		HttpProxy string
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

func (conf *BaseConfig) LoadConfig(confFile, addr, confDir, token string) {
	if confFile == "" {
		confFile = path.Join(BASE_CONF.userDir, "conf.toml")
	}
	_, _ = toml.DecodeFile(confFile, conf)
	if confDir == "" {
		BASE_CONF.ConfigDir = path.Join(BASE_CONF.userDir, "conf")
	} else {
		BASE_CONF.ConfigDir = confDir
	}
	if conf.Period == 0 {
		conf.Period = 3600
	}
	if addr != "" {
		conf.Addr = addr
	}
	if conf.Addr == "" {
		conf.Addr = ":8080"
	}
	if token != "" {
		conf.Token = token
	}

}
func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
		ForceColors:     true,
	})
	u, err := user.Current()
	if err != nil {
		panic("fail to read user dir")
	}
	USER_DIR = u.HomeDir
	confPath := path.Join(USER_DIR, CONF_DIR)
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		err = os.Mkdir(confPath, 0755)
		if err != nil {
			panic(err)
		}
	}
	BASE_CONF = &BaseConfig{
		userDir: confPath,
	}

	ServerAddr := kingpin.Flag("addr", "server addr").Default("").String()
	ConfigDir := kingpin.Flag("config-dir", "config dir contain channel config toml").Default("").Short('c').String()
	HttpToken := kingpin.Flag("token", "token to authenticate").Short('t').Default("").String()
	BaseConf := kingpin.Flag("base-config", "base config").Default("").String()
	kingpin.Parse()
	BASE_CONF.LoadConfig(*BaseConf, *ServerAddr, *ConfigDir, *HttpToken)
	SetProxy(BASE_CONF.HttpProxy)
}

func main() {
	server := common.NewUnixSocketServer(path.Join(BASE_CONF.userDir, SOCKET_FILE))
	if *Cmd != "start" {
		responseBody, err := server.Dial(*Cmd)
		if err != nil {
			logrus.Fatal(err)
		} else {
			logrus.Info(string(responseBody))
		}
		return
	}
	go func() {
		defer server.Stop()
		for {
			select {
			case <-server.Stoped():
				return
			default:
				//do your daemon service
			}
		}
	}()
	go func() {
		if err := server.Listen(); err != nil {
			//stop you daemon if necessary
			logrus.Fatal(err)
		}
	}()
	engine, err := xorm.NewEngine("sqlite3", path.Join(BASE_CONF.userDir, DATAFILE))
	if err != nil {
		logrus.Fatal(err)
	}
	err = engine.CreateTables(&Item{})
	if err != nil {
		logrus.Fatal(err)
	}
	repository := newRepository(engine)

	CONFIG := &Config{}
	CONFIG.LoadConfig(BASE_CONF.ConfigDir)
	if err = CONFIG.Check(repository); err != nil {
		logrus.Fatal(err)
	}
	updateChannel := make(chan struct{})
	server.SetHandler(func(s *common.UnixSocketServer, c net.Conn) error {
		for {
			buf := make([]byte, 512)
			nr, err := c.Read(buf)
			if err != nil {
				return nil
			}

			data := buf[0:nr]
			switch string(data) {
			case "status":
				_, err = fmt.Fprintf(c, "running:%d", os.Getpid())
			case "stop":
				_, err = fmt.Fprintf(c, "stop:%d", os.Getpid())
				s.Stop()
			case "reload":
				_, err = fmt.Fprintf(c, "reload:%d", os.Getpid())
				CONFIG.LoadConfig(BASE_CONF.ConfigDir)
				if err = CONFIG.Check(repository); err != nil {
					logrus.Fatal(err)
				}
				updateChannel <- struct{}{}
			default:
				_, err = fmt.Fprintf(c, "invalid signal")
			}
			if err != nil {
				return err
			}
		}
	})
	go func() {
		for {
			for _, channel := range CONFIG.Channel {
				go func(c *ChannelConf) {
					if err := c.Update(); err != nil {
						logrus.Errorf("update item for %s:%v", c.Desc.Title, err)
					}
				}(channel)
			}
			select {
			case <-updateChannel:
			case <-time.After(time.Second * time.Duration(BASE_CONF.Period)):
			}
		}
	}()
	gin.SetMode("release")
	route := gin.Default()
	route.Use(func(ctx *gin.Context) {
		if BASE_CONF.Token != "" && ctx.Query("token") != BASE_CONF.Token {
			_ = ctx.AbortWithError(403, fmt.Errorf("token is not match"))
		}
	})
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
	if err = route.Run(BASE_CONF.Addr); err != nil {
		logrus.Fatal(err)
	}
}
