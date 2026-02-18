package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
)

//go:embed static
var staticFS embed.FS

func main() {
	addr := ":8080"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	http.Handle("/", http.FileServer(http.FS(sub)))

	fmt.Printf("Listening on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
