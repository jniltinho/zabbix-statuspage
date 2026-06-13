package main

import (
	"embed"
	"os"

	"zabbix-statuspage/cmd"
)

//go:embed web/templates web/static
var embeddedFiles embed.FS

func main() {
	if err := cmd.Execute(embeddedFiles); err != nil {
		os.Exit(1)
	}
}
