package main

import (
	"log"
	"os"

	"github.com/datarobot/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
