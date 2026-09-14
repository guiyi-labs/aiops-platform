// Command aiops-mcp serves the AIOps platform's read-only diagnostic surface to
// a general-purpose AI agent over the Model Context Protocol.
//
// It is a separate process on purpose. The agent's client launches it and
// speaks MCP over stdio, so the platform's own API surface, its authentication
// and its audit trail are all unchanged: this process holds a token and calls
// the same GET endpoints the web console calls. Nothing about the platform's
// attack surface has to grow to accommodate the agent.
//
// Configuration comes from the environment:
//
//	AIOPS_API_URL      base URL of the platform API (required)
//	AIOPS_API_TOKEN    bearer token to send (recommended; most tools need it)
//	AIOPS_API_TIMEOUT  request timeout in Go duration syntax (default 30s)
//
// A client configuration looks like this (Claude Desktop, Cursor and others
// take the same shape):
//
//	{
//	  "mcpServers": {
//	    "aiops": {
//	      "command": "aiops-mcp",
//	      "env": {
//	        "AIOPS_API_URL": "http://127.0.0.1:8080",
//	        "AIOPS_API_TOKEN": "<token>"
//	      }
//	    }
//	  }
//	}
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/mcpserver"
)

// version is the server version reported during initialize. Override at build
// time with -ldflags "-X main.version=<sha>".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "aiops-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := strings.TrimSpace(os.Getenv("AIOPS_API_URL"))
	if baseURL == "" {
		return fmt.Errorf("AIOPS_API_URL is required (for example http://127.0.0.1:8080)")
	}

	token := strings.TrimSpace(os.Getenv("AIOPS_API_TOKEN"))
	if token == "" {
		// Not fatal: an unauthenticated lab deployment is legitimate, and the
		// platform answers 401 through the tool result, which the agent can
		// read and report rather than the server dying at startup.
		fmt.Fprintln(os.Stderr,
			"aiops-mcp: AIOPS_API_TOKEN is empty; tools will return 401 unless the platform allows anonymous reads")
	}

	timeout := 30 * time.Second
	if raw := strings.TrimSpace(os.Getenv("AIOPS_API_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("AIOPS_API_TIMEOUT %q: %w", raw, err)
		}
		timeout = parsed
	}

	backend := mcpserver.NewHTTPBackend(baseURL, token, timeout)
	server := mcpserver.New(backend, mcpserver.DefaultTools(), version)

	// stdout carries the protocol. Anything written there that is not a
	// JSON-RPC message corrupts the stream the client is parsing, so this
	// process must never log to stdout.
	return server.Serve(context.Background(), os.Stdin, os.Stdout)
}
