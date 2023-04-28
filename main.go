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
	APP_NAME     = "web2rss"
	CONF_DIR     = ".config"
	USER_DIR     string
	BASE_CONF    *BaseConfig
	Cmd          = kingpin.Arg("command", "action comand").Required().Enum("start", "stop", "status", "reload", "update", "test")
	CHANNEL_NAME = kingpin.Arg("channel", "command channel target").Default("").String()
	OutputFile   = kingpin.Flag("output", "test output file path").Default("").Short('o').String()
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

func loadChanalConf(path string) (ChannelConf, error) {
	cconf := ChannelConf{}
	_, err := toml.DecodeFile(path, &cconf)
	if err != nil {
		return cconf, fmt.Errorf("read config fail for %s:%v", path, err)
	}
	return cconf, nil
}

func (conf *Config) LoadConfig(dir string, target string) {
	if target != "" {
		cconf, err := loadChanalConf(path.Join(dir, target+".toml"))
		if err != nil {
			logrus.Error(err)
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
		logrus.Infof("load config file: %s", target)
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
			cconf, err := loadChanalConf(path.Join(dir, f.Name()))
			if err != nil {
				logrus.Error(err)
				return
			}
			if err != nil {
				logrus.Error(err)
				continue
			}
			logrus.Infof("load config file: %s", f.Name())
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
		err = os.Mkdir(confPath, 0700)
		if err != nil {
			panic(err)
		}
	}
	confPath = path.Join(USER_DIR, CONF_DIR, APP_NAME)
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		err = os.Mkdir(confPath, 0700)
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
	switch *Cmd {
	case "start":
		break
	case "test":
		if *CHANNEL_NAME == "" {
			logrus.Error("<channel config file> is required for test")
			return
		}
		cconf, err := loadChanalConf(*CHANNEL_NAME)
		if err != nil {
			logrus.Error(err)
			return
		}
		err = cconf.CheckConf(nil)
		if err != nil {
			logrus.Error(err)
			return
		}
		items, err := cconf.Rule.GenerateItem()
		if err != nil {
			logrus.Error(err)
			return
		}
		itemList := make([]Item, len(items))
		for i, d := range items {
			itemList[i] = *d
		}
		rawBody, err := cconf.RssRenderItem(itemList)
		if err != nil {
			logrus.Error(err)
			return
		}
		if *OutputFile != "" {
			outputfile, err := os.OpenFile(*OutputFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0655)
			if err != nil {
				logrus.Fatalf("open output file %s:%v", *OutputFile, err)
			}
			_, err = outputfile.Write(rawBody)
			if err != nil {
				logrus.Fatalf("write output file %s:%v", *OutputFile, err)
			}
			outputfile.Sync()
			err = outputfile.Close()
			if err != nil {
				logrus.Fatalf("close output file %s:%v", *OutputFile, err)
			} else {
				logrus.Info("output file: ", *OutputFile)
			}
		} else {
			fmt.Println(string(rawBody))
		}
		return
	default:
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
			targetChannel = strings.Join(signals,",")
		}
		for _,channelName := range strings.Split(targetChannel,","){	
			CONFIG.LoadConfig(BASE_CONF.ConfigDir, channelName)
			if err = CONFIG.Check(repository); err != nil {
				return err
			}
			repository.ClearCache(targetChannel)
			updateChannel <- CmdSignal{Channel: channelName}
		}
		
		return err
	})
	server.SetSignalHandlerFunc("update", func(s *common.UnixSocketServer, c net.Conn, signals ...string) error {
		_, serr := fmt.Fprintf(c, "update:%d", os.Getpid())
		if err != nil {
			return serr
		}
		targetChannel := ""
		if len(signals) > 0 {
			targetChannel = strings.Join(signals,",")
		}
		for _,channelName := range strings.Split(targetChannel,","){	
			updateChannel <- CmdSignal{Channel: channelName}
		}
		return serr
	})
	channelUpdateSchedule := common.NewSchedule()
	go func() {
		for cmdS := range updateChannel {
			if cmdS.Channel == "" {
				channelUpdateSchedule.Clear()
				for _, channel := range CONFIG.Channel {
					if channel.DBless{
						continue
					}
					channelUpdateSchedule.Add(time.Now().Add(time.Second), channel.Desc.Title)
				}
			} else {
				channelUpdateSchedule.Remove(cmdS.Channel)
				channelUpdateSchedule.Add(time.Now(), cmdS.Channel)
			}
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
				if channelConf.DBless || channelConf.DisableUpdate {
					continue
				}
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
		query := struct {
			SearchKey string `form:"s"`
			PageIndex int    `form:"p"`
			PageSize  int    `form:"size"`
		}{}
		ctx.BindQuery(&query)
		if query.PageIndex < 1 {
			query.PageIndex = 1
		}
		body, err := channel.ToRss(query.SearchKey, query.PageSize, query.PageIndex)
		if err != nil {
			_ = ctx.AbortWithError(500, err)
			return
		}
		ctx.Writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
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
