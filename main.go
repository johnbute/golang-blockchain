package main

import (
	"os"

	"github.com/johnbute/golang-blockchain/cli"
)

func main(){
	defer os.Exit(0)
	cmd := cli.CommandLine{}
	cmd.Run()
}

