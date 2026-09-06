// Command ap-tools is the mock provider behind the tool-call harness: the ten
// accounts-payable tools of the study as HTTP endpoints. Each call applies a
// pretend side effect and leaves the framework-style default log line
// (timestamp, tool, truncated input), which is configuration A's record.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var tools = []string{"list_invoices", "get_invoice", "read_document", "approve_invoice", "send_payment",
	"update_vendor", "send_email", "export_report", "delete_invoice", "run_query"}

func main() {
	port := flag.String("port", envOr("PORT", "8091"), "listen port")
	flag.Parse()
	known := map[string]bool{}
	for _, t := range tools {
		known[t] = true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"healthy"}`)
	})
	// Served on both paths: /tools/{tool} for a direct caller and /v1/tools/{tool}
	// for calls that arrive through aex-gateway, which keeps the path it received.
	invoke := func(w http.ResponseWriter, r *http.Request) {
		tool := r.PathValue("tool")
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if !known[tool] {
			http.Error(w, `{"error":"unknown tool"}`, http.StatusNotFound)
			return
		}
		// The whole of what a framework logs by default: when, which tool, the start of the input.
		input := string(body)
		if len(input) > 40 {
			input = input[:40]
		}
		fmt.Fprintf(os.Stdout, "%s INFO agent.tools invoking %s input=%s...\n", time.Now().UTC().Format(time.RFC3339Nano), tool, input)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tool": tool, "result": "side effect applied"})
	}
	mux.HandleFunc("POST /tools/{tool}", invoke)
	mux.HandleFunc("POST /v1/tools/{tool}", invoke)
	log.Printf("ap-tools listening on :%s with %d tools", *port, len(tools))
	log.Fatal(http.ListenAndServe(":"+*port, mux))
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
