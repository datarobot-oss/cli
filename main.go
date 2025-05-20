package main

import (
	"github.com/datarobot/cli/cmd"
	"log"
	"os"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
