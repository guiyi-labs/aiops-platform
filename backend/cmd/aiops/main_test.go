package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDispatch(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantOutput string
		wantErr    string
	}{
		{name: "no args prints usage", args: nil, wantCode: 2, wantErr: "Usage:"},
		{name: "version short", args: []string{"--version"}, wantCode: 0, wantOutput: "aiops 0.1.0"},
		{name: "version single dash", args: []string{"-version"}, wantCode: 0, wantOutput: "aiops 0.1.0"},
		{name: "version word", args: []string{"version"}, wantCode: 0, wantOutput: "aiops 0.1.0"},
		{name: "help", args: []string{"help"}, wantCode: 0, wantOutput: "Usage:"},
		{name: "unknown command", args: []string{"frobnicate"}, wantCode: 2, wantErr: `unknown command "frobnicate"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("run(%v) code = %d, want %d", tt.args, code, tt.wantCode)
			}
			if tt.wantOutput != "" && !strings.Contains(stdout.String(), tt.wantOutput) {
				t.Fatalf("stdout = %q, want contains %q", stdout.String(), tt.wantOutput)
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Fatalf("stderr = %q, want contains %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestVersionConstantMatchesRelease(t *testing.T) {
	if version != "0.1.0" {
		t.Fatalf("version = %q, want 0.1.0 (must track the v0.1.0 release tag)", version)
	}
}
