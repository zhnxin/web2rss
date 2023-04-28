package main

import (
	"strings"
	"time"

	"encoding/xml"

	"github.com/patrickmn/go-cache"
	"xorm.io/xorm"
)

type (
	Item struct {
		Id          int64     `xml:"-"`
		Mk          string    `xml:"-" xorm:"'mk' text notnull unique(mk_channel)"`
		Title       *RssCdata    `xml:"title" xorm:"'title' text"`
		Link        *RssCdata    `xml:"link" xorm:"'link' text"`
		Guid        *RssCdata    `xml:"guid" xorm:"'guid' text"`
		Category 	*RssCdata    `xml:"category" xorm:"'category' text"`
		PubDate     time.Time `xml:"pubDate" xorm:"'pubDate' DATETIME"`
		Description *RssCdata `xml:"description" xorm:"'description' text"`
		Thumb       string    `xml:"thumb,omitempty" xorm:"'thumb' text"`
		Channel     string    `xml:"-" xorm:"'channel' text unique(mk_channel)"`
		ukey        string    `xml:"-" xorm:"-"`
	}
	Repository struct {
		keySetCache *cache.Cache
		engine      *xorm.Engine
	}
	RssRoot struct{
		XMLName xml.Name `xml:"rss"`
		Version string `xml:"version,attr"`
		Atom string `xml:"xmlns:atom,attr"`
		Channel RssChannel `xml:"channel"`
	}
	RssChannel struct{
		Title *RssCdata `xml:"title"`
		Language string `xml:"language"`
		PubDate time.Time `xml:"pubDate,omitempty"`
		Generator *RssCdata `xml:"generator,omitempty"`
		Description *RssCdata `xml:"description,omitempty"`
		Link string `xml:"link"`
		Item []Item `xml:"item"`
	}
	RssCdata struct{
		Content string `xml:",cdata"`
	}
)
func (c *RssCdata) FromDB(bytes []byte) error {
	if len(bytes) > 0{
		*c = *newRssCdata(string(bytes))
	}
	return nil
}
func (c *RssCdata) ToDB() ([]byte, error) {
	return []byte(c.Content), nil
}
func (c *RssCdata) String() string {
	return c.Content
}
func (*Item) TableName() string { return "item" }
func (i *Item) Key() string {
	if i.ukey == "" {
		i.ukey = i.Channel + ":" + i.Mk
	}
	return i.ukey
}
func clearItem(items []*Item) []*Item {
	if len(items) < 1 {
		return nil
	}
	itemSet := map[string]*Item{}
	for _, i := range items {
		if _, ok := itemSet[i.Key()]; !ok {
			itemSet[i.Key()] = i
		}
	}
	clearItems := []*Item{}
	for _, v := range itemSet {
		clearItems = append(clearItems, v)
	}
	return clearItems
}

func newRepository(engine *xorm.Engine) *Repository {
	return &Repository{engine: engine, keySetCache: cache.New(time.Hour*24, time.Hour*12)}
}

func (r *Repository) Exists(channel, key string) (bool, error) {
	cacheKey := channel + ":" + key
	_, ok := r.keySetCache.Get(cacheKey)
	if ok {
		return true, nil
	}
	ok, err := r.engine.Where("mk = ? and channel = ?", key, channel).Exist(&Item{})
	if err == nil && ok {
		r.keySetCache.Set(cacheKey, true, cache.DefaultExpiration)
	}
	return ok, err
}
func(r *Repository) ClearCache(channel string){
	cacheKey := channel + ":"
	for k,_ := range r.keySetCache.Items(){
		if strings.HasPrefix(k, cacheKey){
			r.keySetCache.Delete(k)
		}
	}
}

func (r *Repository) FindItem(channel, searchKey string, pageSize, pageIndex int) ([]Item, error) {
	query := r.engine.Table(&Item{})
	if searchKey != "" {
		query = query.Where("channel = ? AND title LIKE ?", channel, "%"+searchKey+"%")
	} else {
		query = query.Where("channel = ?", channel)
	}
	if pageSize < 1 {
		query = query.Limit(20, (pageIndex-1)*20)
	} else {
		query = query.Limit(pageSize, (pageIndex-1)*pageSize)
	}
	items := []Item{}
	err := query.Desc("pubDate").Find(&items)
	return items, err
}

func (r *Repository) Save(items []*Item) error {
	if len(items) < 1 {
		return nil
	}
	_, err := r.engine.Insert(items)
	if err == nil {
		for _, i := range items {
			r.keySetCache.Set(i.Channel+":"+i.Mk, true, cache.DefaultExpiration)
		}
	}
	return err
}

func newRssCdata(content string) *RssCdata{
	if len(content) > 0{
		return &RssCdata{Content: content}
	}
	return nil
}

func NewRssChannel(title,language,generator,description,link string,pubDate time.Time,items []Item)([]byte, error){
	rssRoot := RssRoot{
		Version: "2.0",
		Atom: "http://www.w3.org/2005/Atom",
		Channel:RssChannel{
			Title: newRssCdata(title),
			Language: language,
			PubDate: pubDate,
			Generator: newRssCdata(generator),
			Description: newRssCdata(description),
			Link: link,
			Item: items,
		},
	}
	return xml.Marshal(rssRoot)
}