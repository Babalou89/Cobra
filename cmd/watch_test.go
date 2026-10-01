package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartWatchServerServesState(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cage"), 0o755); err != nil {
		t.Fatal(err)
	}
	url, shutdown := startWatchServer(dir, 0, true)
	defer shutdown()
	if !strings.HasPrefix(url, "http://") {
		t.Fatalf("url = %q", url)
	}
	resp, err := http.Get(url + "api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("api/state not JSON: %v: %s", err, body)
	}
	if _, ok := payload["events"]; !ok {
		t.Fatalf("payload missing events: %s", body)
	}
	r2, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatalf("dashboard status %d", r2.StatusCode)
	}
}

func TestStartWatchServerPortBusyDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	url1, shutdown1 := startWatchServer(dir, 0, true)
	defer shutdown1()
	// Reuse the exact port: second server must degrade gracefully.
	port := url1[strings.LastIndex(url1[:len(url1)-1], ":")+1 : len(url1)-1]
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	_, shutdown2 := startWatchServer(dir, p, true)
	shutdown2()
}
