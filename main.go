package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/parnurzeal/gorequest"
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
	Cmd          = kingpin.Arg("command", "action comand").Required().Enum("start", "stop", "status", "reload", "update","ws","log", "test")
	CHANNEL_NAME = kingpin.Arg("channel", "command channel target").Default("").String()
	OutputFile   = kingpin.Flag("output", "test output file path").Default("").Short('o').String()
	WS_UPGRADER = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	   }
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
		AdminToken string
		ConfigDir string
		userDir   string
		Period    int
		HttpProxy string
		LogLevel  string
	}
	CmdSignal struct {
		Channel string
	}
	CmdResponseDto struct {
		ErrCode int `json:"err_code"`
		Message string `json:"message"`
		Data int `json:"data"`
	}
	CmdRequestDto struct{
		Cmd string `json:"cmd"`
		Args string `json:"args"`
		Token string `json:"token"`
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
			if err != nil {
				LOGGER.Error(err)
				continue
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

func checkHealth() (int,error){
	respBody := CmdResponseDto{}
	_, _, errs := gorequest.New().Get("http://"+BASE_CONF.Addr+"/health").
		Set("Content-Type", "application/json").
		EndStruct(&respBody)
	if len(errs) > 0{
		return 0,errs[0]
	}
	return respBody.Data,nil
}
func do_command(cmd string,args string)(CmdResponseDto,error){
	cmdBody := CmdRequestDto{
		Cmd: cmd,
		Args: args,
		Token: BASE_CONF.AdminToken,
	}
	respBody := CmdResponseDto{}
	_, _, errs := gorequest.New().Put("http://"+BASE_CONF.Addr+"/web2rss/signal").
			Set("Content-Type", "application/json").
			Send(cmdBody).
			EndStruct(&respBody)
	var err error
	if len(errs) > 0{
		err = errs[0]
	}
	return respBody,err
}

func handleWSClient(){
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: BASE_CONF.Addr, Path: "/web2rss/ws"}
	LOGGER.Infof("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		LOGGER.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})
	reviceChannel := make(chan []byte)
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				fmt.Println("read:", err)
				return
			}
			reviceChannel <- message
		}
	}()
	scanner := bufio.NewScanner(os.Stdin)
	inputChannel := make(chan string)
	go func(){
		for scanner.Scan(){
			inputChannel <- scanner.Text()
		}
	}()
	for {
		select {
		case <-done:
			return
		case t := <-inputChannel:
			err := c.WriteMessage(websocket.TextMessage, []byte(t))
			if err != nil {
				LOGGER.Fatal("write:", err)
				return
			}
		case m := <-reviceChannel:
			fmt.Print(string(m))
		case <-interrupt:
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				fmt.Println("write close:", err)
				return
			}
			LOGGER.Info("exit……")
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
func initFunc() {
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
	switch *Cmd {
	case "start":
		break
	case "test":
		if *CHANNEL_NAME == "" {
			LOGGER.Error("<channel config file> is required for test")
			return
		}
		cconf, err := loadChanalConf(*CHANNEL_NAME)
		if err != nil {
			LOGGER.Error(err)
			return
		}
		err = cconf.CheckConf(nil)
		if err != nil {
			LOGGER.Error(err)
			return
		}
		items, err := cconf.Rule.GenerateItem()
		if err != nil {
			LOGGER.Error(err)
			return
		}
		itemList := make([]Item, len(items))
		for i, d := range items {
			itemList[i] = *d
		}
		rawBody, err := cconf.RssRenderItem(itemList)
		if err != nil {
			LOGGER.Error(err)
			return
		}
		if *OutputFile != "" {
			outputfile, err := os.OpenFile(*OutputFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0655)
			if err != nil {
				LOGGER.Fatalf("open output file %s:%v", *OutputFile, err)
			}
			_, err = outputfile.Write(rawBody)
			if err != nil {
				LOGGER.Fatalf("write output file %s:%v", *OutputFile, err)
			}
			outputfile.Sync()
			err = outputfile.Close()
			if err != nil {
				LOGGER.Fatalf("close output file %s:%v", *OutputFile, err)
			} else {
				LOGGER.Info("output file: ", *OutputFile)
			}
		} else {
			fmt.Println(string(rawBody))
		}
		return
	case "status":
		pid,err := checkHealth()
		if err != nil {
			fmt.Println("web2rss is not alive")
			os.Exit(1)
		}else{
			logrus.Infof("PID: %d",pid)
		}
		return	
	case "ws","log":
		handleWSClient()
		return
	default:
		resp,err := do_command(*Cmd,*CHANNEL_NAME)
		if err != nil{
			LOGGER.Fatalf("%v",err)
		}else{
			if(resp.ErrCode != 0){
				LOGGER.Fatalf("%s",resp.Message)
			}else{
				LOGGER.Info(resp.Message)
			}
		}
		return
	}
	engine, err := xorm.NewEngine("sqlite3", path.Join(BASE_CONF.userDir, DATAFILE))
	if err != nil {
		LOGGER.Fatal(err)
	}
	repository := newRepository(engine)

	CONFIG := &Config{}
	CONFIG.LoadConfig(BASE_CONF.ConfigDir, "")
	if err = CONFIG.Check(repository); err != nil {
		LOGGER.Fatal(err)
	}
	updateChannel := make(chan CmdSignal)
	channelUpdateSchedule := common.NewSchedule()
	go func() {
		for cmdS := range updateChannel {
			if cmdS.Channel == "" {
				channelUpdateSchedule.Clear()
				for _, channel := range CONFIG.Channel {
					if channel.DBless {
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
						LOGGER.Errorf("update item for %s:%v", c.Desc.Title, err)
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
	func_reload := func(channelList string) error{
		for _, channelName := range strings.Split(channelList, ",") {
			CONFIG.LoadConfig(BASE_CONF.ConfigDir, channelName)
			if err = CONFIG.Check(repository); err != nil {
				return err
			}
			repository.ClearCache(channelName)
			updateChannel <- CmdSignal{Channel: channelName}
		}
		return nil;
	}
	func_update := func(channelList string){
		for _, channelName := range strings.Split(channelList, ",") {
			updateChannel <- CmdSignal{Channel: channelName}
		}
	}
	func_stop := func(){
			time.Sleep(time.Microsecond * 100)
			LOGGER.Infof("web2rss(%d): 停止服务",os.Getpid())
			os.Exit(0)
		}
	gin.SetMode("release")
	gin.DefaultWriter = LOGGER.Writer()
	route := gin.Default()
	route.Use(func(ctx *gin.Context) {
		if BASE_CONF.Token != "" && ctx.Query("token") != BASE_CONF.Token {
			_ = ctx.AbortWithError(403, fmt.Errorf("token is not match"))
		}
	})
	route.GET("health", func(ctx *gin.Context){
		ctx.JSON(200, gin.H{"err_code":0,"message":"ok","data":os.Getpid()})
	})
	route.PUT("web2rss/signal", func(ctx *gin.Context){
		reqBody := CmdRequestDto{}
		err := ctx.BindJSON(&reqBody)
		if err != nil {
			_ = ctx.AbortWithError(400, err)
			return
		}
		switch reqBody.Cmd {
		case "reload":
			err := func_reload(reqBody.Args)
			if err != nil {
				_ = ctx.AbortWithError(400, err)
				return
			}
			ctx.JSON(200, gin.H{"err_code":0,"message":"ok","data":os.Getpid()})
		case "update":
			func_update(reqBody.Args)
			ctx.JSON(200, gin.H{"err_code":0,"message":"ok","data":os.Getpid()})
		case "stop":
			go func_stop()
			ctx.JSON(200, gin.H{"err_code":0,"message":"ok","data":os.Getpid()})
		default:
			ctx.JSON(400, gin.H{"err_code":400,"message":"unknown cmd","data":os.Getpid()})
		}
	})
	route.GET("web2rss/ws",func(ctx *gin.Context) {
		conn, err := WS_UPGRADER.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			LOGGER.Error(err)
			return
		}
		connKey := fmt.Sprintf("%v",conn)
		LOGGER.Infof("add websoket client: %s",connKey)
		LOGGER.AddWriter(connKey,conn)
		defer func(){
			LOGGER.RemoveWriter(connKey)
			conn.Close()
			LOGGER.Infof("close websoket client: %s",connKey)
		}()
		for{
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					LOGGER.Error(err)
				}
				return
			}
			cmd := string(msg)
			if cmd == ""{
				conn.WriteMessage(websocket.TextMessage, []byte(">> cmd is empty\n"))
				continue
			}
			cmdargs := strings.Split(cmd," ")
			cmd = cmdargs[0]
			args := ""
			if len(cmdargs) > 1{
				args = cmdargs[1]
			}
			switch cmd {
				case "reload","r":
					err :=func_reload(args)
					if err!=nil{
						_ = ctx.AbortWithError(400, err)
						conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(">> 重新加载配置异常: %v\n",err)))
					}else{
						conn.WriteMessage(websocket.TextMessage, []byte(">> ok\n"))
					}
				case "update","u":
					func_update(args)
					conn.WriteMessage(websocket.TextMessage, []byte(">> ok\n"))
				case "stop":
					go func_stop()
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					conn.WriteMessage(websocket.TextMessage, []byte(">> ok\n"))
					return
				case "alive","status":
					conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(">> pid: %d\n",os.Getpid())))
				case "exit":
					conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return
				default:
					conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(">> cmd %s not support\n", cmd)))
			}
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
	route.GET("/html/:channel", func(ctx *gin.Context) {
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
		items, err := channel.Find(query.SearchKey, query.PageSize, query.PageIndex)
		if err != nil {
			_ = ctx.AbortWithError(500, err)
			return
		}
		tmpl, err := template.New("channelTableHtml").Parse(channelTableHtml)
		if err != nil {
			LOGGER.Error(err)
			ctx.JSON(500, gin.H{"err": err.Error()})
			return
		}
		_ = tmpl.Execute(ctx.Writer, items)
	})
	route.GET("/html/:channel/:id", func(ctx *gin.Context) {
		channelName := ctx.Param("channel")
		channel, ok := CONFIG.channelMap[channelName]
		if !ok {
			_ = ctx.AbortWithError(404, fmt.Errorf("channelName %s not found", channelName))
			return
		}
		idStr := ctx.Param("id")
		if err != nil {
			_ = ctx.AbortWithError(500, err)
			return
		}
		item, err := channel.FindByMk(ctx.Param("channel"),idStr)
		if err != nil {
			_ = ctx.AbortWithError(500, err)
			return
		}
		if item.Id == 0 {
			ctx.Status(404)
			ctx.Writer.WriteString(fmt.Sprintf(itemNotFoundPage, channel.Rule.channel,channel.Desc.Title))
			return
		}
		tmpl, err := template.New("itemDetailHtml").Parse(itemDetailHtml)
		if err != nil {
			LOGGER.Error(err)
			ctx.JSON(500, gin.H{"err": err.Error()})
			return
		}
		_ = tmpl.Execute(ctx.Writer, item)
	})
	route.GET("/schedule", func(ctx *gin.Context) {
		if strings.Contains(ctx.GetHeader("Accept"), "text/html") {
			ctx.Status(200)
			tmpl, err := template.New("htmlTest").Parse(htmlTmpl)
			if err != nil {
				LOGGER.Error(err)
				ctx.JSON(500, gin.H{"err": err.Error()})
				return
			}
			_ = tmpl.Execute(ctx.Writer, channelUpdateSchedule.GetSchedule())
		} else {
			ctx.JSON(200, channelUpdateSchedule.GetSchedule())
		}
	})
	LOGGER.Infof("web2rss 开始服务: %d",os.Getpid())
	if err = route.Run(BASE_CONF.Addr); err != nil {
		LOGGER.Fatal(err)
	}
}