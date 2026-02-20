package main

import (
	"context"
	"embed"
	"encoding/json"
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

	show, err := loadMockShow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading show: %v\n", err)
		os.Exit(1)
	}

	timeline, err := BuildTimeline(show)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building timeline: %v\n", err)
		os.Exit(1)
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, show)
	})
	mux.HandleFunc("/api/timeline", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, timeline)
	})

	if len(runAndExit) > 0 {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		srv := &http.Server{Handler: mux}
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
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func loadMockShow() (Show, error) {
	buf, err := staticFS.ReadFile("static/show.json")
	if err != nil {
		return Show{}, err
	}
	var show Show
	if err := json.Unmarshal(buf, &show); err != nil {
		return Show{}, err
	}
	return show, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
