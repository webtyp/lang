package main

import (
	"fmt"
	"os"

	"webtyp.com/lang/langc"
	"webtyp.com/modfind"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Println("Usage: langc sync [-tool] [dir]")
		os.Exit(0)
	}

	if os.Args[1] != "sync" {
		fmt.Println("Usage: langc sync [-tool] [dir]")
		os.Exit(0)
	}

	toolMode := false
	dir := "."

	args := os.Args[2:]
	if len(args) > 0 && args[0] == "-tool" {
		toolMode = true
		args = args[1:]
	}

	if len(args) > 0 {
		dir = args[0]
	}

	log := func(args ...any) {
		fmt.Fprintln(os.Stderr, args...)
	}

	t := langc.New(modfind.New(), log)

	var err error
	if toolMode {
		err = t.SyncToolTranslations(dir)
	} else {
		err = t.SyncTranslations(dir)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
