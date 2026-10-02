package main

import (
	"os"

	"github.com/eureka-corp/md-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
