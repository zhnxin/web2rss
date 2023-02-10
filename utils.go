package main

import (
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
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

func generateTemplate(tempName, tempContext string) (*template.Template, error) {
	return template.New(tempName).Funcs(sprig.TxtFuncMap()).Funcs(map[string]interface{}{
		"timeFromStr": tmplFuncDateFromStr,
		"timeToStr":   tmpFuncDateToStr,
		"isList":tmpFuncIsList,
	}).Parse(tempContext)
}
