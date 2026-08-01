package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomeHandlerRendersPage(t *testing.T) {
	handler := NewHomeHandler()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	handler.Get(recorder, request)

	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf(
			"status code = %d, want %d",
			response.StatusCode,
			http.StatusOK,
		)
	}

	body := recorder.Body.String()

	if !strings.Contains(body, "<h1>KinOps</h1>") {
		t.Errorf("response body does not contain KinOps heading")
	}

	if !strings.Contains(body, "Your household operations hub is running.") {
		t.Errorf("response body does not contain expected status text")
	}
}
