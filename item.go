package main

import (
	"time"

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
	}
	Repository struct {
		engine *xorm.Engine
	}
)

func (*Item) TableName() string { return "item" }

func newRepository(engine *xorm.Engine) *Repository {
	return &Repository{engine: engine}
}

func (r *Repository) Exists(channel, key string) (bool, error) {
	return r.engine.Where("mk = ? and channel = ?", key, channel).Exist(&Item{})
}

func (r *Repository) FindItem(channel string, limit int) ([]Item, error) {
	items := []Item{}
	err := r.engine.Where("channel = ?", channel).Desc("pubDate").Limit(limit, 0).Find(&items)
	return items, err
}

func (r *Repository) Save(items []Item) error {
	if len(items) < 1 {
		return nil
	}
	_, err := r.engine.Insert(items)
	return err
}
