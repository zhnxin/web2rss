package main

import (
	"regexp"
	"testing"
)

func TestSelector(t *testing.T) {
	regexRes := regexp.MustCompile(`(\d{4}-\d+-\d+ \d+:\d+)`).FindStringSubmatch(`本文由&nbsp;
	     &nbsp;于&nbsp;2022-2-15 8:12 发布在&nbsp;
		 `)
	if len(regexRes) > 1 {
		t.Log(regexRes[1])
		ts :=tmplFuncDateFromStr("Y-m-d H:M",regexRes[1])
		t.Logf("%+v",ts)
		t.Log(tmpFuncDateToStr("RFC3339",ts))
	}
}

func TestCurrentTimeDesc(t *testing.T){
	testT := func(timeDesc string){
		tr,err :=currentBeforeCn(timeDesc)
		if err !=nil{
			t.Error(err.Error())
		}
		t.Logf("%s → %s",timeDesc,tmpFuncDateToStr("2006-m-d H:M:SS",tr))
	}
	testT("2021-8-4")
	testT("2021-8-19")
}