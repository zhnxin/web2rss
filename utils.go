package main

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/sirupsen/logrus"
)

var (
	xmlEncodeMap = map[string]string{
		"<":  "&lt;",
		">":  "&gt;",
		"'":  "&apos;",
		"\"": "&quot;",
		"&":  "&amp;",
	}
	formatMap map[string]string = map[string]string{
		"YY": "06",
		"Y":  "2006",
		"m":  "1",
		"mm": "01",
		"d":  "2",
		"dd": "02",
		"H":  "15",
		"M":  "4",
		"S":  "5",
		"MM": "04",
		"SS": "05",
	}
)

func EncodeStrForXml(s string) string {
	var tmpStr = s
	for k, v := range xmlEncodeMap {
		tmpStr = strings.ReplaceAll(tmpStr, k, v)
	}
	return tmpStr
}

func tmpGenerateTimeFormat(layer string) string {
	formatLayer := layer
	if layer == "rfc3339" || layer == "RFC3339" || layer == "" {
		return time.RFC3339
	}
	for _, k := range []string{"YY", "Y", "m", "d", "H", "M", "S"} {
		if len(k) > 1 {
			formatLayer = strings.ReplaceAll(formatLayer, k, formatMap[k])
		} else {
			p := regexp.MustCompile(k + "+")
			formatLayer = p.ReplaceAllString(formatLayer, formatMap[k])
		}
	}
	return formatLayer
}

func tmplFuncDateFromStr(layer, value string) time.Time {
	formatLayer := tmpGenerateTimeFormat(layer)
	ts, _ := time.Parse(formatLayer, value)
	return ts
}

func tmpFuncDateToStr(layer string, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(tmpGenerateTimeFormat(layer))
}

func tmpFuncIsList(value interface{}) bool {
	switch reflect.TypeOf(value).Kind() {
	case reflect.Slice:
		return true
	default:
		return false
	}
}

func currentBeforeCn(timeDesc string) (time.Time,error){
	if len(timeDesc) < 1{
		return time.Now(),fmt.Errorf("getCurrentBeforeCn: 空字符串")
	}
	pattern := regexp.MustCompile(`^(\d+)([^\-\d])`)
	matchers := pattern.FindAllStringSubmatch(timeDesc,-1)
	if len(matchers) == 1 && len(matchers[0]) == 3{
		currentTime := time.Now()
		num,err := strconv.Atoi(matchers[0][1])
		if err != nil{
			logrus.Errorf("getCurrentBeforeCn: 转换失败：%s: %s",timeDesc,err.Error())
			return currentTime,fmt.Errorf("getCurrentBeforeCn: 转换失败：%s: %s",timeDesc,err.Error())
		}
		if matchers[0][2] == "秒"{
			return currentTime.Add(- time.Second * time.Duration(num)),nil
		}else if matchers[0][2] == "分"{
			return currentTime.Add(- time.Minute * time.Duration(num)),nil
		}else if matchers[0][2] == "小" || matchers[0][2] == "时"{
			return currentTime.Add(- time.Hour * time.Duration(num)),nil
		}else if matchers[0][2] == "天"{
			return currentTime.AddDate(0, 0, - num),nil
		}else if matchers[0][2] == "月"{
			return currentTime.AddDate(0, - num, 0),nil
		}
		logrus.Errorf("getCurrentBeforeCn: 转换失败：%s",timeDesc)
		return currentTime,fmt.Errorf("getCurrentBeforeCn: 转换失败：%s: %s",timeDesc,err.Error())
	}
	dataPattern := regexp.MustCompile(`\d+-\d+-\d+`)
	if dataPattern.MatchString(timeDesc){
		return tmplFuncDateFromStr("2006-m-d",timeDesc),nil
	}
	logrus.Errorf("getCurrentBeforeCn: 转换失败：%s",timeDesc)
	return time.Now(),fmt.Errorf("转换失败：未匹配任一规则: %s",timeDesc )
	
}

func generateTemplate(tempName, tempContext string) (*template.Template, error) {
	return template.New(tempName).Funcs(sprig.TxtFuncMap()).Funcs(map[string]interface{}{
		"timeFromStr": tmplFuncDateFromStr,
		"timeToStr":   tmpFuncDateToStr,
		"currentBeforeCn":currentBeforeCn,
		"isList":tmpFuncIsList,
	}).Parse(tempContext)
}
