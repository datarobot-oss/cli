package main

import (
	"datarobot/cli/dr"
	_ "datarobot/cli/dr/auth"
	_ "datarobot/cli/dr/templates"
)

func main() {
	dr.Execute()
}
