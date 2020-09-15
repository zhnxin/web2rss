package main

import "strings"

func EncodeStrForXml(s string) string {
	return strings.ReplaceAll(
		strings.ReplaceAll(
			strings.ReplaceAll(
				strings.ReplaceAll(
					strings.ReplaceAll(s, "<", "&lt;"),
					">", "&gt;"),
				"'", "&apos;"),
			"\"", "&quot;"),
		"&", "&amp;")
}
