package main

import (
	"fmt"
	"strings"
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
func (i *Item) ToInsertSql() string {
	return fmt.Sprintf("('%s','%s','%s','%s','%s','%s','%s','%s')", i.Mk, i.Title, i.Link, i.Guid, i.PubDate.Format("2006-01-02 15:04:05"), i.Description, i.Thumb, i.Channel)
}

func newRepository(engine *xorm.Engine) *Repository {
	return &Repository{engine: engine}
}

func (r *Repository) Exists(channel, key string) (bool, error) {
	return r.engine.Where("mk = ? and channel = ?", key, channel).Exist(&Item{})
}

func (r *Repository) FindItem(channel string, limit int) ([]Item, error) {
	if limit == 0 {
		limit = 20
	}
	items := []Item{}
	err := r.engine.Where("channel = ?", channel).Desc("pubDate").Limit(limit, 0).Find(&items)
	return items, err
}

func (r *Repository) Save(items []Item) error {
	if len(items) < 1 {
		return nil
	}
	lines := make([]string, len(items))
	for i, d := range items {
		lines[i] = d.ToInsertSql()
	}
	_, err := r.engine.Exec(
		`INSERT INTO ` + (new(Item).TableName()) + " (mk,title,link,guid,pubDate,description,thumb,channel) values" +
			strings.Join(lines, ",") +
			" ON CONFLICT(mk) DO UPDATE SET pubDate=excluded.pubDate;")
	return err
}
