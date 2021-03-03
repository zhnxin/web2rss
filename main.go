package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path"
	"strings"
	"text/template"
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
	DATAFILE     = "data.db"
	CONF_DIR     = ".web2rss"
	USER_DIR     string
	BASE_CONF    *BaseConfig
	Cmd          = kingpin.Arg("command", "action comand").Required().Enum("start", "stop", "status", "reload", "update")
	CHANNEL_NAME = kingpin.Arg("channel", "command channel target").Default("").String()
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
		LogLevel  string
	}
	CmdSignal struct {
		Channel string
	}
)

func (conf *Config) Get(channel string) (*ChannelConf, bool) {
	if conf.channelMap == nil {
		conf.channelMap = map[string]*ChannelConf{}
		for _, c := range conf.Channel {
			conf.channelMap[c.Desc.Title] = c
		}
	}
	c, ok := conf.channelMap[channel]
	return c, ok
}

func (conf *Config) Check(repository *Repository) error {
	conf.channelMap = map[string]*ChannelConf{}
	ok, err := repository.engine.IsTableExist(new(Item))
	if err != nil {
		return err
	}
	if !ok {
		err = repository.engine.CreateTables(new(Item))
		if err != nil {
			return err
		}
		err = repository.engine.CreateUniques(new(Item))
		if err != nil {
			return err
		}
	}
	for _, c := range conf.Channel {
		err := c.CheckConf(repository)
		if err != nil {
			return err
		}
		conf.channelMap[c.Desc.Title] = c
	}
	return nil
}

func (conf *Config) LoadConfig(dir string, target string) {
	if target != "" {
		filePath := path.Join(dir, target+".toml")
		cconf := ChannelConf{}
		_, err := toml.DecodeFile(filePath, &cconf)
		if err != nil {
			logrus.Errorf("read config fail for %s:%v", filePath, err)
			return
		}
		isUpdate := false
		for i, d := range conf.Channel {
			if d.Desc.Title == target {
				conf.Channel[i] = &cconf
				isUpdate = true
			}
		}
		if !isUpdate {
			conf.Channel = append(conf.Channel, &cconf)
		}
		logrus.Infof("load config file: %s", filePath)
		return
	}
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
	switch conf.LogLevel {
	case "DEBUG", "debug", "D", "d":
		logrus.SetLevel(logrus.DebugLevel)
	case "INFO", "I", "i", "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "ERROR", "E", "e", "error", "ERR", "err":
		logrus.SetLevel(logrus.ErrorLevel)
	}

}
func initFunc() {
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
	initFunc()
	server := common.NewUnixSocketServer(path.Join(BASE_CONF.userDir, SOCKET_FILE))
	if *Cmd != "start" {
		responseBody, err := server.Dial(*Cmd + " " + *CHANNEL_NAME)
		if err != nil {
			logrus.Fatal(err)
		} else {
			logrus.Info(string(responseBody))
		}
		return
	}
	go func() {
		<-server.Stoped()
		os.Exit(0)
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
	repository := newRepository(engine)

	CONFIG := &Config{}
	CONFIG.LoadConfig(BASE_CONF.ConfigDir, "")
	if err = CONFIG.Check(repository); err != nil {
		logrus.Fatal(err)
	}
	updateChannel := make(chan CmdSignal)
	server.SetSignalHandlerFunc("reload", func(s *common.UnixSocketServer, c net.Conn, signals ...string) error {
		_, err := fmt.Fprintf(c, "reload:%d", os.Getpid())
		if err != nil {
			return err
		}
		targetChannel := ""
		if len(signals) > 0 {
			targetChannel = signals[0]
		}
		CONFIG.LoadConfig(BASE_CONF.ConfigDir, targetChannel)
		if err = CONFIG.Check(repository); err != nil {
			return err
		}
		updateChannel <- CmdSignal{Channel: targetChannel}
		return err
	})
	server.SetSignalHandlerFunc("update", func(s *common.UnixSocketServer, c net.Conn, signals ...string) error {
		_, serr := fmt.Fprintf(c, "update:%d", os.Getpid())
		targetChannel := ""
		if len(signals) > 0 {
			targetChannel = signals[0]
		}
		updateChannel <- CmdSignal{Channel: targetChannel}
		return serr
	})
	channelUpdateSchedule := common.NewSchedule()
	go func() {
		for cmdS := range updateChannel {
			channelUpdateSchedule.Remove(cmdS.Channel)
			channelUpdateSchedule.Add(time.Now(), cmdS.Channel)
		}
	}()
	for _, channel := range CONFIG.Channel {
		channelUpdateSchedule.Add(time.Now().Add(time.Second), channel.Desc.Title)
	}
	go func() {
		for targetChannel := range channelUpdateSchedule.Chan() {
			channelName := targetChannel.(string)
			channelConf, ok := CONFIG.Get(channelName)
			if ok {
				go func(c *ChannelConf) {
					if err := c.Update(); err != nil {
						logrus.Errorf("update item for %s:%v", c.Desc.Title, err)
					}
				}(channelConf)
				if channelConf.Period > 0 {
					channelUpdateSchedule.Add(time.Now().Add(time.Duration(channelConf.Period)*time.Second), channelName)
				} else {
					channelUpdateSchedule.Add(time.Now().Add(time.Duration(BASE_CONF.Period)*time.Second), channelName)
				}
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
	route.GET("/schedule", func(ctx *gin.Context) {
		if strings.Contains(ctx.GetHeader("Accept"), "text/html") {
			ctx.Status(200)
			tmpl, err := template.New("htmlTest").Parse(htmlTmpl)
			if err != nil {
				logrus.Error(err)
				ctx.JSON(500, gin.H{"err": err.Error()})
				return
			}
			_ = tmpl.Execute(ctx.Writer, channelUpdateSchedule.GetSchedule())
		} else {
			ctx.JSON(200, channelUpdateSchedule.GetSchedule())
		}
	})
	if err = route.Run(BASE_CONF.Addr); err != nil {
		logrus.Fatal(err)
	}
}
