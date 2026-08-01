package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dcrespo1/kinops/internal/auth"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestWritePasswordHash(t *testing.T) {
	var output bytes.Buffer
	if err := writePasswordHash(strings.NewReader("secret\nignored"), &output); err != nil {
		t.Fatal(err)
	}
	manager, err := auth.NewManager("admin", strings.TrimSpace(output.String()), false)
	if err != nil {
		t.Fatalf("generated hash is invalid: %v", err)
	}
	if !manager.Authenticate("admin", "secret") {
		t.Error("generated hash does not verify the input password")
	}
	if err := writePasswordHash(strings.NewReader("\n"), &bytes.Buffer{}); err == nil {
		t.Error("empty password was accepted")
	}
}

func TestCheckHealth(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "http://127.0.0.1:8081/healthz" {
			t.Errorf("request URL = %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("ok\n"))}, nil
	})}
	if err := checkHealth(client, "127.0.0.1:8081"); err != nil {
		t.Fatalf("checkHealth() error = %v", err)
	}
}

func TestCheckHealthRejectsUnhealthyResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Body: io.NopCloser(strings.NewReader("unhealthy"))}, nil
	})}
	if err := checkHealth(client, "127.0.0.1:8081"); err == nil {
		t.Fatal("checkHealth() returned nil error")
	}
}
