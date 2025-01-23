set GOARCH=amd64
set GOOS=windows
go build -ldflags="-s -w -H windowsgui" -o EMInit_v1.1.exe main.go
upx --best --lzma EMInit_v1.1.exe