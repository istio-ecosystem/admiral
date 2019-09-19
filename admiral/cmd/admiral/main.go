package main

import (
	"github.com/admiral/admiral/cmd/admiral/cmd"
	"os"
)

func main() {
	rootCmd := cmd.GetRootCmd(os.Args[1:])

	if err := rootCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}
