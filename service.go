package main

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sirupsen/logrus"
	common "github.com/zhnxin/common-go"
	"github.com/zouyx/agollo/v3/component/log"
)

type (
	CmdSignal struct {
		Channel string
	}
	Config struct {
		Channel    []*ChannelConf
		channelMap map[string]*ChannelConf
	}
	BaseConfig struct {
		Addr       string
		Token      string
		AdminToken string
		ConfigDir  string
		userDir    string
		Period     int
		HttpProxy  string
		LogLevel   string
	}
	ChannelStatus struct {
		Item   string    `json:"item"`
		T      time.Time `json:"t"`
		Update bool      `json:"is_update"`
	}
	Service struct {
		repository    *Repository
		channel       *Config
		schedule      *common.Schedule
		updateChannel chan CmdSignal
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
			LOGGER.Error(err)
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
		LOGGER.Infof("load config file: %s", target)
		return
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Error(err)
		return
	}
	conf.Channel = []*ChannelConf{}
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".toml") {
			cconf, err := loadChanalConf(path.Join(dir, f.Name()))
			if err != nil {
				LOGGER.Error(err)
				return
			}
			LOGGER.Infof("load config file: %s", f.Name())
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
		LOGGER.SetLevel(logrus.DebugLevel)
	case "INFO", "I", "i", "info":
		LOGGER.SetLevel(logrus.InfoLevel)
	case "ERROR", "E", "e", "error", "ERR", "err":
		LOGGER.SetLevel(logrus.ErrorLevel)
	}
}

func (svc *Service) updateScheduleDaemon() {
	for cmdS := range svc.updateChannel {
		if cmdS.Channel == "" {
			svc.schedule.Clear()
			for _, channel := range svc.channel.Channel {
				if channel.DBless {
					continue
				}
				svc.schedule.Add(time.Now().Add(time.Second), channel.Desc.Title)
			}
		} else {
			svc.schedule.Remove(cmdS.Channel)
			svc.schedule.Add(time.Now(), cmdS.Channel)
		}
	}
}
func (svc *Service) scheduleDaemon() {
	for targetChannel := range svc.schedule.Chan() {
		channelName := targetChannel.(string)
		channelConf, ok := svc.channel.Get(channelName)
		if !ok {
			continue
		}
		if channelConf.DBless || channelConf.DisableUpdate {
			continue
		}
		if channelConf.Rule.isRunning() {
			LOGGER.Infof("channel %s is running, skip this schedule", channelName)
			svc.schedule.Add(time.Now().Add(time.Duration(BASE_CONF.Period)*time.Second), channelName)
			continue
		}
		go func(c *ChannelConf) {
			if err := c.Update(); err != nil {
				LOGGER.Errorf("update item for %s:%v", c.Desc.Title, err)
			}
		}(channelConf)
		if channelConf.Period > 0 {
			svc.schedule.Add(time.Now().Add(time.Duration(channelConf.Period)*time.Second), channelName)
		} else {
			svc.schedule.Add(time.Now().Add(time.Duration(BASE_CONF.Period)*time.Second), channelName)
		}
	}
}

func (svc *Service) Reload(channelList string) error {
	for _, channelName := range strings.Split(channelList, ",") {
		svc.channel.LoadConfig(BASE_CONF.ConfigDir, channelName)
		if err := svc.channel.Check(svc.repository); err != nil {
			return err
		}
		svc.repository.ClearCache(channelName)
		svc.updateChannel <- CmdSignal{Channel: channelName}
	}
	return nil
}
func (svc *Service) Update(channelList string) {
	for _, channelName := range strings.Split(channelList, ",") {
		svc.updateChannel <- CmdSignal{Channel: channelName}
	}
}
func (svc *Service) GetChannel(channel string) (*ChannelConf, bool) {
	return svc.channel.Get(channel)
}
func (svc *Service) GetChannelNameList() []string {
	names := make([]string, len(svc.channel.Channel))
	for i, channel := range svc.channel.Channel {
		names[i] = channel.Desc.Title
	}
	sort.Strings(names)
	return names
}

func (svc *Service) GetChannelStatus() []ChannelStatus {
	scheduleList := svc.schedule.GetSchedule()
	channelInfoList := make([]ChannelStatus, len(scheduleList))
	for i, ch := range scheduleList {
		channelName := ch.Item.(string)
		channelConf, ok := svc.channel.Get(channelName)
		update := false
		if ok && channelConf.Rule.isRunning() {
			update = true
		}
		channelInfoList[i] = ChannelStatus{
			Item:   channelName,
			T:      ch.T,
			Update: update,
		}
	}
	// Sort channelInfoList by Item
	sort.Slice(channelInfoList, func(i, j int) bool {
		return channelInfoList[i].Item < channelInfoList[j].Item
	})
	return channelInfoList
}

func NewService(repository *Repository, config *Config) *Service {
	svc := &Service{
		repository:    repository,
		channel:       config,
		schedule:      common.NewSchedule(),
		updateChannel: make(chan CmdSignal, 100),
	}
	go svc.updateScheduleDaemon()
	for _, channel := range svc.channel.Channel {
		svc.schedule.Add(time.Now().Add(time.Second), channel.Desc.Title)
	}
	go svc.scheduleDaemon()
	return svc
}
