package main

import (
	"fmt"
	"os"

	"webtyp.com/lang/langc"
	"webtyp.com/modfind"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Println("Usage: langc sync [dir]")
		os.Exit(0)
	}

	if os.Args[1] != "sync" {
		fmt.Println("Usage: langc sync [dir]")
		os.Exit(0)
	}

	dir := "."
	if len(os.Args) > 2 {
		dir = os.Args[2]
	}

	log := func(args ...any) {
		fmt.Fprintln(os.Stderr, args...)
	}

	t := langc.New(modfind.New(), log)
	if err := t.SyncTranslations(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
