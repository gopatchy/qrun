package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

//go:embed static
var staticFS embed.FS

func main() {
	addr := ":8080"
	var runAndExit []string

	for _, arg := range os.Args[1:] {
		if v, ok := strings.CutPrefix(arg, "--run-and-exit="); ok {
			runAndExit = strings.Fields(v)
		} else {
			addr = arg
		}
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	http.Handle("/", http.FileServer(http.FS(sub)))

	if len(runAndExit) > 0 {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		srv := &http.Server{}
		go srv.Serve(ln)

		cmd := exec.Command(runAndExit[0], runAndExit[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmdErr := cmd.Run()
		srv.Shutdown(context.Background())
		if cmdErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", cmdErr)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("Listening on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
