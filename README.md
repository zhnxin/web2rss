# web2rss

## Introduction

This application is running as a spider. It get the rss items by css selector.

## Quick Start
```
go get github.com/zhnxin/web2rss
web2rss --addr :8080 -c <config dir>
```

## Config

### Rule.KeyParseConf
Key Needed to catch in the toc page
### Rule.ExtraKeyParseConf
Get Key from url defind in ExtraSource.
### Rule.TemplateConfig
Item attribute