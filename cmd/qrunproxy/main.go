package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
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
	addr := flag.String("addr", ":8080", "listen address")
	runAndExitStr := flag.String("run-and-exit", "", "command to run after server starts, then exit")
	printTimeline := flag.Bool("print-timeline-and-exit", false, "print timeline JSON and exit")
	flag.Parse()

	var runAndExit []string
	if *runAndExitStr != "" {
		runAndExit = strings.Fields(*runAndExitStr)
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

	if *printTimeline {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(timeline); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
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
		ln, err := net.Listen("tcp", *addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		port := fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
		srv := &http.Server{Handler: mux}
		go srv.Serve(ln)

		for i, arg := range runAndExit {
			runAndExit[i] = strings.ReplaceAll(arg, "{port}", port)
		}
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

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Listening on %s\n", ln.Addr())
	if err := http.Serve(ln, mux); err != nil {
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
