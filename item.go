package main

import (
	"time"

	"github.com/patrickmn/go-cache"
	"xorm.io/xorm"
)

type (
	Item struct {
		Mk          string    `xml:"-" xorm:"'mk' text pk "`
		Title       string    `xml:"title" xorm:"'title' text"`
		Link        string    `xml:"link" xorm:"'link' text"`
		Guid        string    `xml:"guid" xorm:"'guid' text"`
		PubDate     time.Time `xml:"pubDate" xorm:"'pubDate' DATETIME"`
		Description string    `xml:"description" xorm:"'description' text"`
		Thumb       string    `xml:"thumb,omitempty" xorm:"'thumb' text"`
		Channel     string    `xml:"-" xorm:"'channel' text"`
		ukey        string    `xml:"-" xorm:"-"`
	}
	Repository struct {
		keySetCache *cache.Cache
		engine      *xorm.Engine
	}
)

func (*Item) TableName() string { return "item" }
func (i *Item) Key() string {
	if i.ukey == "" {
		i.ukey = i.Channel + ":" + i.Mk
	}
	return i.ukey
}
func (*Item) CreateTablesSql() string {
	return "CREATE TABLE `item` (`mk` TEXT NOT NULL, `title` TEXT NULL, `link` TEXT NULL, `guid` TEXT NULL, `pubDate` DATETIME NULL, `description` TEXT NULL, `thumb` TEXT NULL, `channel` TEXT NULL,PRIMARY KEY (mk,channel));"
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

func (r *Repository) FindItem(channel string, limit int) ([]Item, error) {
	if limit == 0 {
		limit = 20
	}
	items := []Item{}
	err := r.engine.Where("channel = ?", channel).Desc("pubDate").Limit(limit, 0).Find(&items)
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
