package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"xorm.io/xorm"
)

var (
	DATAFILE     = "data.db"
	APP_NAME     = "web2rss"
	CONF_DIR     = ".config"
	USER_DIR     string
	BASE_CONF    *BaseConfig
	Cmd          = kingpin.Arg("command", "action comand").Required().Enum("start", "stop", "status", "reload", "update", "ws", "log", "test")
	CHANNEL_NAME = kingpin.Arg("channel", "command channel target").Default("").String()
	OutputFile   = kingpin.Flag("output", "test output file path").Default("").Short('o').String()
	WS_UPGRADER  = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	SELF_CLIENT *req.Client
	SERVICE     *Service
)

type (
	CmdResponseDto struct {
		ErrCode int    `json:"err_code"`
		Message string `json:"message"`
		Data    int    `json:"data"`
	}
	CmdRequestDto struct {
		Cmd   string `json:"cmd"`
		Args  string `json:"args"`
		Token string `json:"token"`
	}
	Controller struct {
		service *Service
	}
)

func checkHealth() (int, error) {
	if SELF_CLIENT == nil {
		SELF_CLIENT = req.NewClient()
	}
	respBody := CmdResponseDto{}
	_, err := SELF_CLIENT.R().SetHeader("Content-Type", "application/json").SetSuccessResult(&respBody).
		Get("http://" + BASE_CONF.Addr + "/health")
	if err != nil {
		return 0, err
	}
	return respBody.Data, nil
}
func do_command(cmd string, args string) (CmdResponseDto, error) {
	if SELF_CLIENT == nil {
		SELF_CLIENT = req.NewClient()
	}
	cmdBody := CmdRequestDto{
		Cmd:   cmd,
		Args:  args,
		Token: BASE_CONF.AdminToken,
	}
	respBody := CmdResponseDto{}
	_, err := SELF_CLIENT.R().
		SetBody(cmdBody).
		SetHeader("Content-Type", "application/json").SetSuccessResult(&respBody).
		Put("http://" + BASE_CONF.Addr + "/web2rss/signal")
	return respBody, err
}

func handleWSClient() {
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
	go func() {
		for scanner.Scan() {
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
		channelTest(*CHANNEL_NAME)
		return
	case "status":
		pid, err := checkHealth()
		if err != nil {
			fmt.Println("web2rss is not alive")
			os.Exit(1)
		} else {
			logrus.Infof("PID: %d", pid)
		}
		return
	case "ws", "log":
		handleWSClient()
		return
	default:
		resp, err := do_command(*Cmd, *CHANNEL_NAME)
		if err != nil {
			LOGGER.Fatalf("%v", err)
		} else {
			if resp.ErrCode != 0 {
				LOGGER.Fatalf("%s", resp.Message)
			} else {
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

	ruleConfig := &Config{}
	ruleConfig.LoadConfig(BASE_CONF.ConfigDir, "")
	if err = ruleConfig.Check(repository); err != nil {
		LOGGER.Fatal(err)
	}
	service := NewService(repository, ruleConfig)
	gin.SetMode("release")
	gin.DefaultWriter = LOGGER.Writer()
	route := gin.Default()
	route.Use(func(ctx *gin.Context) {
		if BASE_CONF.Token != "" && ctx.Query("token") != BASE_CONF.Token {
			_ = ctx.AbortWithError(403, fmt.Errorf("token is not match"))
		}
	})
	controller := &Controller{service: service}
	route.GET("health", controller.GetHealth)
	route.GET("web2rss", controller.GetInfo)
	route.PUT("web2rss/signal", controller.HandleSignal)
	route.GET("web2rss/ws", controller.HandleWS)
	route.GET("/rss", controller.GetRss)
	route.GET("/rss/:channel", controller.GetRssChannel)
	route.GET("/html", controller.GetHtmlChannelList)
	route.GET("/html/:channel", controller.GetHtmlChannel)
	route.GET("/html/:channel/:id", controller.GetHtmlChannelItem)
	LOGGER.Infof("web2rss 开始服务: %d", os.Getpid())
	if err = route.Run(BASE_CONF.Addr); err != nil {
		LOGGER.Fatal(err)
	}
}
func channelTest(channelName string) {
	if channelName == "" {
		LOGGER.Error("<channel config file> is required for test")
		return
	}
	cconf, err := loadChanalConf(channelName)
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
	output, err := json.Marshal(itemList)
	if err != nil {
		LOGGER.Error(err)
		return
	}
	LOGGER.Infoln("生成条目：")
	LOGGER.Infoln(string(output))
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
}

func mainStop() {
	time.Sleep(time.Microsecond * 100)
	LOGGER.Infof("web2rss(%d): 停止服务", os.Getpid())
	os.Exit(0)
}

func (c *Controller) GetHealth(ctx *gin.Context) {
	ctx.JSON(200, gin.H{"err_code": 0, "message": "ok", "data": os.Getpid()})
}
func (c *Controller) GetInfo(ctx *gin.Context) {
	channelInfoList := c.service.GetChannelStatus()
	ctx.JSON(200, gin.H{"data": channelInfoList, "status": 0, "message": "ok"})
}

func (c *Controller) HandleSignal(ctx *gin.Context) {
	reqBody := CmdRequestDto{}
	err := ctx.BindJSON(&reqBody)
	if err != nil {
		_ = ctx.AbortWithError(400, err)
		return
	}
	switch reqBody.Cmd {
	case "reload":
		err := c.service.Reload(reqBody.Args)
		if err != nil {
			_ = ctx.AbortWithError(400, err)
			return
		}
		ctx.JSON(200, gin.H{"err_code": 0, "message": "ok", "data": os.Getpid()})
	case "update":
		c.service.Update(reqBody.Args)
		ctx.JSON(200, gin.H{"err_code": 0, "message": "ok", "data": os.Getpid()})
	case "stop":
		go mainStop()
		ctx.JSON(200, gin.H{"err_code": 0, "message": "ok", "data": os.Getpid()})
	default:
		ctx.JSON(400, gin.H{"err_code": 400, "message": "unknown cmd", "data": os.Getpid()})
	}
}

func (c *Controller) HandleWS(ctx *gin.Context) {
	conn, err := WS_UPGRADER.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		LOGGER.Error(err)
		return
	}
	connKey := MD5Hash(fmt.Sprintf("%v", conn))
	LOGGER.Infof("add websoket client: %s", connKey)
	LOGGER.AddWriter(connKey, conn)
	defer func() {
		LOGGER.RemoveWriter(connKey)
		conn.Close()
		LOGGER.Infof("close websoket client: %s", connKey)
	}()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				LOGGER.Error(err)
			}
			return
		}
		cmd := string(msg)
		if cmd == "" {
			conn.WriteMessage(websocket.TextMessage, []byte(">> cmd is empty\n"))
			continue
		}
		cmdargs := strings.Split(cmd, " ")
		cmd = cmdargs[0]
		args := ""
		if len(cmdargs) > 1 {
			args = cmdargs[1]
		}
		switch cmd {
		case "reload", "r":
			err := c.service.Reload(args)
			if err != nil {
				_ = ctx.AbortWithError(400, err)
				conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(">> 重新加载配置异常: %v\n", err)))
			} else {
				conn.WriteMessage(websocket.TextMessage, []byte(">> ok\n"))
			}
		case "update", "u":
			c.service.Update(args)
			conn.WriteMessage(websocket.TextMessage, []byte(">> ok\n"))
		case "info", "i":
			channelStatus := c.service.GetChannelStatus()
			body, err := json.MarshalIndent(channelStatus, "", "  ")
			if err != nil {
				conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("获取频道信息失败：%+v\n", err)))
			} else {
				conn.WriteMessage(websocket.TextMessage, body)
			}
		case "stop":
			go mainStop()
			conn.WriteMessage(websocket.CloseMessage, []byte{})
			conn.WriteMessage(websocket.TextMessage, []byte(">> ok\n"))
			return
		case "alive", "status":
			conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(">> pid: %d\n", os.Getpid())))
		case "exit":
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		default:
			conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(">> cmd %s not support\n", cmd)))
		}
	}
}
func (c *Controller) GetRss(ctx *gin.Context) {
	ctx.JSON(200, gin.H{"rss": c.service.GetChannelNameList()})
}
func (c *Controller) GetRssChannel(ctx *gin.Context) {
	channelName := ctx.Param("channel")
	channel, ok := c.service.GetChannel(channelName)
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
}
func (c *Controller) GetHtmlChannelList(ctx *gin.Context) {
	channelInfoList := c.service.GetChannelStatus()
	ctx.Status(200)
	tmpl, err := template.New("htmlTest").Parse(htmlTmpl)
	if err != nil {
		LOGGER.Error(err)
		ctx.JSON(500, gin.H{"err": err.Error()})
		return
	}
	_ = tmpl.Execute(ctx.Writer, channelInfoList)
}

func (c *Controller) GetHtmlChannel(ctx *gin.Context) {
	channelName := ctx.Param("channel")
	channel, ok := c.service.GetChannel(channelName)
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
}

func (c *Controller) GetHtmlChannelItem(ctx *gin.Context) {
	channelName := ctx.Param("channel")
	channel, ok := c.service.GetChannel(channelName)
	if !ok {
		_ = ctx.AbortWithError(404, fmt.Errorf("channelName %s not found", channelName))
		return
	}
	idStr := ctx.Param("id")
	item, err := channel.FindByMk(ctx.Param("channel"), idStr)
	if err != nil {
		_ = ctx.AbortWithError(500, err)
		return
	}
	if item.Id == 0 {
		ctx.Status(404)
		ctx.Writer.WriteString(fmt.Sprintf(itemNotFoundPage, channel.Rule.channel, channel.Desc.Title))
		return
	}
	tmpl, err := template.New("itemDetailHtml").Parse(itemDetailHtml)
	if err != nil {
		LOGGER.Error(err)
		ctx.JSON(500, gin.H{"err": err.Error()})
		return
	}
	_ = tmpl.Execute(ctx.Writer, item)
}
