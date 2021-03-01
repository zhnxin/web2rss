package main

import "strings"

var (
	xmlEncodeMap = map[string]string{
		"<":  "&lt;",
		">":  "&gt;",
		"'":  "&apos;",
		"\"": "&quot;",
		"&":  "&amp;",
	}
)

func EncodeStrForXml(s string) string {
	var tmpStr = s
	for k, v := range xmlEncodeMap {
		tmpStr = strings.ReplaceAll(tmpStr, k, v)
	}
	return tmpStr
}
