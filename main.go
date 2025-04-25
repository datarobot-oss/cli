package main

import (
	"github.com/datarobot/cli/dr"
	_ "github.com/datarobot/cli/dr/auth"
	_ "github.com/datarobot/cli/dr/templates"
)

func main() {
	dr.Execute()
}
