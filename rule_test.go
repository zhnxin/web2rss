package main

import (
	"regexp"
	"testing"
)

func TestSelector(t *testing.T) {
	regexRes := regexp.MustCompile(`<a href="(.*)".*?>.*?</a>`).FindStringSubmatch(`ad  asd<a href="/asdfadfdfd.html">safadfafd</a>`)
	if len(regexRes) > 1 {
		t.Log(regexRes[1])
	}
}
