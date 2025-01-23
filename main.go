package main

import "EMInit/internal/tool"

func main() {
	t := tool.NewFirmwareFlashTool()
	t.Run()
}
