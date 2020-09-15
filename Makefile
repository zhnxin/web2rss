compile:
	echo 'Compiling for everu Os and platform'
	GOOS=linux GOARCH=amd64 go build -o bin/web2rss_linux_amd64
	GOOS=windows GOARCH=amd64 go build -o bin/web2rss_win_amd64.exe
	GOOS=darwin GOARCH=amd64 go build -o bin/web2rss_mac_amd64

