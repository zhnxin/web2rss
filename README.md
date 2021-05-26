# web2rss

## Introduction

This application is running as a spider. It get the rss items by css selector.

## Quick Start
```
go get github.com/zhnxin/web2rss
web2rss --addr :8080 -c <config dir>
```

## Usage

THe config location is ~/.config/web2rss.

```
# As for zsh, this file is .zprofile
cat <<EOT >> $HOME/.bash_profile

function web2rss(){
  local cmd=$1
  local arg=$2
  case ${cmd} in
    start)
      nohup $HOME/go/bin/web2rss start > ${HOME}/.config/web2rss/log.log 2>&1 &
      ;;
    stop|status)
      $HOME/go/bin/web2rss $cmd
      ;;
    test|update|reload)
       $HOME/go/bin/web2rss $cmd $arg
       ;;
    logs)
      tail -fn128 ${HOME}/.config/web2rss/log.log
      ;;
    *)
      $HOME/go/bin/web2rss --help
  esac
}
EOT
```

## Config

### Rule.KeyParseConf
Key Needed to catch in the toc page
### Rule.ExtraKeyParseConf
Get Key from url defind in ExtraSource.
### Rule.TemplateConfig
Item attribute