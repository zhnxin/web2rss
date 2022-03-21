package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/PuerkitoBio/goquery"
	"github.com/parnurzeal/gorequest"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var (
	proxyUrl       string
	rssTemplate, _ = template.New("RssTemplate").Funcs(sprig.TxtFuncMap()).Funcs(map[string]interface{}{
		"timeFromStr": tmplFuncDateFromStr,
		"timeToStr":   tmpFuncDateToStr,
	}).Parse(`
<rss xmlns:atom="http://www.w3.org/2005/Atom" version="2.0">
	<channel>
	<title>{{.Desc.Title}}</title>
	<language>{{.Desc.Language}}</language>
	<pubDate>{{.PutDate}}</pubDate>
	{{if .Desc.Generator}}<generator>{{.Desc.Generator}}</generator>{{end}}
	{{if .Desc.Description}}<generator>{{.Desc.Description}}</generator>{{end}}
	{{if .Desc.Image}}<image><url>{{.Desc.Image}}</url><title>{{.Desc.Title}}</title><link>{{.Desc.Link}}</link></image>{{end}}
	<link>{{.Desc.Link}}</link>
	{{range $i,$element := .Items }}<item>
		<title>{{$element.Title}}</title>
		<pubDate>{{$element.PubDate }}</pubDate>
		<link>{{$element.Link}}</link>
		<guid>{{$element.Guid}}</guid>
		<thumb>{{$element.Thumb}}</thumb>
		<description>
		<![CDATA[ {{$element.Description}} ]]>
		</description>
	</item>
	{{end}}
	</channel>
</rss>`)
)

func SetProxy(proxy string) {
	proxyUrl = proxy
}

type (
	ChannelData struct {
		Desc    FeedDesc
		Items   []Item
		PutDate time.Time
	}
	ChannelConf struct {
		ItemCount int
		Period    int

		Desc FeedDesc
		Rule Rule
	}
	FeedDesc struct {
		Title       string
		Url         string
		Image       string
		Description string
		Language    string
		Generator   string
		Link        string
	}
	Rule struct {
		Encoding           string
		TocUrl             string
		TocUrlList         []string
		ItemSelector       string
		ExtraSource        string
		Headers            map[string]string
		ExtraSourceHeaders map[string]string
		NoProxy            bool
		Key                string
		ExtraConfig        map[string]string
		KeyParseConf       map[string]ElementSelector
		ExtraKeyParseConf  map[string]ElementSelector
		TemplateConfig     ItemTemplate
		itemTemplate       *template.Template
		channel            string
		repository         *Repository
	}
	JsonApiSource struct {
		Link         string
		KeyParseConf map[string]JsonElementSelector
	}
	ItemTemplate struct {
		Link        string
		Title       string
		Guid        string
		Description string
		PubDate     string
		Thumbnail   string
	}
	ElementSelector struct {
		Selector string
		Regex    string
		Attr     string
	}
	JsonElementSelector struct {
		Regex   string
		KeyPath []string
	}
)

func NewElementSelector(selector, attr, regex string) ElementSelector {
	return ElementSelector{
		Selector: selector,
		Regex:    regex,
		Attr:     attr,
	}
}
func (e *ElementSelector) getKey(s *goquery.Selection) string {
	var text string
	element :=s
	if e.Selector != "" {
		element = s.Find(e.Selector).First();
		if element == nil{
			logrus.Error("sub element not found for ", e.Selector)
		}
	}
	switch e.Attr {
	case "html":
		text, _ = element.Html()
	case "text", "":
		text = element.Text()
	default:
		var isExists bool
		text, isExists = element.Attr(e.Attr)
		if !isExists {
			logrus.Error("element and atrr not found in extra page for ", e.Selector)
		}
	}
	if e.Regex != "" {
		regexRes := regexp.MustCompile(e.Regex).FindStringSubmatch(text)
		if len(regexRes) > 1 {
			text = regexRes[1]
		}
	}
	if e.Attr != "html" {
		return EncodeStrForXml(text)
	} else {
		return text
	}
}
func (e *ElementSelector) getKeyFromDoc(s *goquery.Document) interface{} {
	res := []string{}
	var regexP *regexp.Regexp
	if e.Regex != "" {
		regexP = regexp.MustCompile(e.Regex)
	}
	s.Find(e.Selector).Each(func(i int, es *goquery.Selection) {
		var text string
		switch e.Attr {
		case "html":
			text, _ = es.Html()
		case "text", "":
			text = es.Text()
		default:
			var isExists bool
			text, isExists = es.Attr(e.Attr)
			if !isExists {
				logrus.Error("element and atrr not found in extra page for ", e.Selector)
				return
			}
		}
		if regexP != nil {
			regexRes := regexP.FindStringSubmatch(text)
			if len(regexRes) > 1 {
				text = regexRes[1]
			} else {
				logrus.Debug(text)
			}
		}
		if e.Attr != "html" {
			res = append(res, EncodeStrForXml(text))
		} else {
			res = append(res, text)
		}
	})
	switch len(res) {
	case 0:
		return ""
	case 1:
		return res[0]
	default:
		return res
	}
}

func (t *ItemTemplate) ToTempalte(templateName string) (*template.Template, error) {
	guid := t.Guid
	if guid == "" {
		guid = t.Link
	}
	thumb := ""
	if t.Thumbnail != "" {
		thumb = fmt.Sprintf("\n<thumb>%s</thumb>", t.Thumbnail)
	}
	templateText := fmt.Sprintf(`<item>
	<title>%s</title>
	<link>%s</link>
	<guid>%s</guid>%s
	<pubDate>%s</pubDate>
	<description>
	<![CDATA[%s]]>
	</description>
</item>`, t.Title, t.Link, guid, thumb, t.PubDate, t.Description)
	return template.New(templateName).Funcs(sprig.TxtFuncMap()).Funcs(map[string]interface{}{
		"timeFromStr": tmplFuncDateFromStr,
		"timeToStr":   tmpFuncDateToStr,
	}).Parse(templateText)
}

func (r *Rule) generateReqClient(url string, isExtraReq bool) *gorequest.SuperAgent {
	req := gorequest.New()
	if !r.NoProxy {
		req = req.Proxy(proxyUrl)
	}
	req = req.Get(url)
	if isExtraReq {
		if len(r.ExtraSourceHeaders) > 0 {
			for k, v := range r.ExtraSourceHeaders {
				req = req.Set(k, v)
			}
		}
	} else {
		if len(r.Headers) > 0 {
			for k, v := range r.Headers {
				req = req.Set(k, v)
			}
		}
	}
	return req
}

func (r *Rule) GenerateItem() ([]*Item, error) {
	var err error
	var extraUrlTmp *template.Template
	if r.ExtraSource != "" {
		extraUrlTmp, err = template.New("").Funcs(sprig.TxtFuncMap()).Parse(r.ExtraSource)
		if err != nil {
			return nil, fmt.Errorf("generate template for extraUrl fail:%v", err)
		}
	}
	tocSet := map[string]bool{r.TocUrl: true}
	for _, u := range r.TocUrlList {
		tocSet[u] = true
	}

	items := []*Item{}

	var doc *goquery.Document
	for tocUrl := range tocSet {
		if tocUrl == "" {
			continue
		}
		logrus.Debug("toc url: ", tocUrl)
		req := r.generateReqClient(tocUrl, false)
		res, _, errs := req.End()
		if len(errs) > 0 {
			return nil, fmt.Errorf("request to toc url fail:%v", errs)
		}

		switch strings.ToLower(r.Encoding) {
		case "gbk", "gb10830":
			doc, err = goquery.NewDocumentFromReader(transform.NewReader(res.Body,
				simplifiedchinese.GB18030.NewDecoder()))
		default:
			doc, err = goquery.NewDocumentFromReader(res.Body)
		}
		if err != nil {
			return nil, fmt.Errorf("parse toc page to document fail:%v", err)
		}

		wait := new(sync.WaitGroup)
		doc.Find(r.ItemSelector).Each(func(i int, s *goquery.Selection) {
			wait.Add(1)
			go func(selection *goquery.Selection) {
				defer wait.Done()
				item := map[string]interface{}{}
				for k, v := range r.ExtraConfig {
					item[k] = v
				}
				for k, selector := range r.KeyParseConf {
					item[k] = selector.getKey(s)
				}
				if r.repository != nil {
					isExists, err := r.repository.Exists(r.channel, fmt.Sprint(fmt.Sprint(item[r.Key])))
					if err != nil {
						logrus.Error(err)
						return
					}
					if isExists {
						return
					}
				}
				if extraUrlTmp != nil {
					var tpl bytes.Buffer
					err := extraUrlTmp.Execute(&tpl, item)
					if err != nil {
						logrus.Error(err)
					} else {
						extraReq := r.generateReqClient(tpl.String(), true)
						extraRes, _, errs := extraReq.End()
						if len(errs) > 0 {
							logrus.Error(errs)
						} else {
							var extraDoc *goquery.Document
							switch strings.ToLower(r.Encoding) {
							case "gbk", "gb10830":
								extraDoc, err = goquery.NewDocumentFromReader(transform.NewReader(extraRes.Body,
									simplifiedchinese.GB18030.NewDecoder()))
							default:
								extraDoc, err = goquery.NewDocumentFromReader(extraRes.Body)
							}
							if err != nil {
								logrus.Error(err)
							} else {
								for k, selector := range r.ExtraKeyParseConf {
									item[k] = selector.getKeyFromDoc(extraDoc)
								}
							}

						}

					}
				}
				var tpl bytes.Buffer
				err = r.itemTemplate.Execute(&tpl, item)
				if err != nil {
					logrus.Error(err)
					return
				}
				itemEntity := &Item{}
				err = xml.Unmarshal(tpl.Bytes(), &itemEntity)
				if err != nil {
					logrus.Errorf("decode item temp fail:%v:\n%s", err, tpl.String())
					return
				}
				itemEntity.Mk = fmt.Sprint(item[r.Key])
				itemEntity.Channel = r.channel
				items = append(items, itemEntity)
			}(s)
		})
		wait.Wait()
	}
	return clearItem(items), nil
}

func NewChannelConf(d FeedDesc,
	r Rule,
	repository *Repository) (*ChannelConf, error) {
	tmpl, err := r.TemplateConfig.ToTempalte(d.Title)
	if err != nil {
		return nil, err
	}
	r.itemTemplate = tmpl
	r.channel = d.Title
	r.repository = repository
	return &ChannelConf{
		Rule: r,
		Desc: d,
	}, nil
}

func (c *ChannelConf) CheckConf(repository *Repository) error {
	tmpl, err := c.Rule.TemplateConfig.ToTempalte(c.Desc.Title)
	if err != nil {
		return err
	}
	c.Rule.itemTemplate = tmpl
	c.Rule.channel = c.Desc.Title
	c.Rule.repository = repository
	return nil
}

func (c *ChannelConf) Update() error {
	res, err := c.Rule.GenerateItem()
	if err != nil {
		return fmt.Errorf("update item %v", err)
	}
	logrus.Infof("update %d for %s", len(res), c.Desc.Title)
	err = c.Rule.repository.Save(res)
	if err != nil {
		err = fmt.Errorf("store data fail:%v", err)
	}
	return err
}

func (c *ChannelConf) ToRss(searchKey string, pageSize, pageIndex int) ([]byte, error) {
	limit := c.ItemCount
	if pageSize > 0 {
		limit = pageSize
	}
	items, err := c.Rule.repository.FindItem(c.Rule.channel, searchKey, limit, pageIndex)
	if err != nil {
		return nil, err
	}
	return c.RssRenderItem(items)
}

func (c *ChannelConf) RssRenderItem(items []Item) ([]byte, error) {
	data := ChannelData{}
	data.Desc = c.Desc
	data.Items = items
	if len(items) > 0 {
		data.PutDate = items[0].PubDate
	}
	var tpl bytes.Buffer
	err := rssTemplate.Execute(&tpl, data)
	if err != nil {
		return nil, err
	}
	return tpl.Bytes(), nil
}
