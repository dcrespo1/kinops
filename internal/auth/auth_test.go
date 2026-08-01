package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashAndAuthentication(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "correct horse") || !strings.HasPrefix(hash, passwordAlgorithm+"$") {
		t.Errorf("unsafe or malformed hash = %q", hash)
	}
	manager, err := NewManager("admin", hash, false)
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Authenticate("admin", "correct horse battery staple") {
		t.Error("valid credentials were rejected")
	}
	for _, credentials := range [][2]string{{"wrong", "correct horse battery staple"}, {"admin", "wrong"}, {"", ""}} {
		if manager.Authenticate(credentials[0], credentials[1]) {
			t.Errorf("invalid credentials %#v were accepted", credentials)
		}
	}
	if _, err := NewManager("admin", "not-a-hash", false); err == nil {
		t.Error("invalid password hash was accepted")
	}
}

func TestSessionLifecycleCSRFAndCookieSecurity(t *testing.T) {
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	manager, err := newManager("admin", hash, true, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	created := httptest.NewRecorder()
	csrfToken, err := manager.CreateSession(created)
	if err != nil {
		t.Fatal(err)
	}
	cookies := created.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/admin" {
		t.Errorf("session cookie = %#v", cookie)
	}
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.AddCookie(cookie)
	if got, ok := manager.CSRFToken(request); !ok || got != csrfToken {
		t.Errorf("CSRFToken() = %q, %v", got, ok)
	}
	if !manager.VerifyCSRF(request, csrfToken) || manager.VerifyCSRF(request, "wrong") {
		t.Error("CSRF verification returned the wrong result")
	}
	called := false
	protected := manager.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	protected.ServeHTTP(httptest.NewRecorder(), request)
	if !called {
		t.Error("valid session did not reach protected handler")
	}

	now = now.Add(SessionTTL)
	expired := httptest.NewRecorder()
	protected.ServeHTTP(expired, request)
	if expired.Code != http.StatusSeeOther || expired.Header().Get("Location") != "/admin/login" {
		t.Errorf("expired response = %d %s", expired.Code, expired.Header().Get("Location"))
	}

	now = now.Add(-SessionTTL)
	created = httptest.NewRecorder()
	_, err = manager.CreateSession(created)
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	request.AddCookie(created.Result().Cookies()[0])
	destroyed := httptest.NewRecorder()
	manager.DestroySession(destroyed, request)
	if destroyed.Result().Cookies()[0].MaxAge != -1 {
		t.Errorf("destroy cookie = %#v", destroyed.Result().Cookies()[0])
	}
	if _, ok := manager.CSRFToken(request); ok {
		t.Error("destroyed session remains active")
	}
}

func TestRequireRedirectsMissingSessionWithoutCaching(t *testing.T) {
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager("admin", hash, false)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	manager.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/admin/login" {
		t.Errorf("response = %d %s", recorder.Code, recorder.Header().Get("Location"))
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("security headers = %#v", recorder.Header())
	}
}
