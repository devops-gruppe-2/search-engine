package main

// Tests af session-cookien (Harden Yourself)

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// loginCookie logger alice ind og giver Set-Cookie-headeren fra svaret
func loginCookie(t *testing.T, app *testApp, forwardedProto string) string {
	t.Helper()
	app.register("alice", "correct-horse")

	form := url.Values{"username": {"alice"}, "password": {"correct-horse"}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.server.URL+"/api/login", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if forwardedProto != "" {
		req.Header.Set("X-Forwarded-Proto", forwardedProto)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	expectStatus(t, response{status: res.StatusCode}, http.StatusOK)
	return res.Header.Get("Set-Cookie")
}

func TestSessionCookieIsSecureOverHTTPS(t *testing.T) {
	app := newTestApp(t)
	cookie := loginCookie(t, app, "https")

	for _, want := range []string{"user_session=", "Secure", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("cookie %q is missing %q", cookie, want)
		}
	}
}

// Kald direkte på IP-adressen over http (fx simulatoren) skal stadig kunne logge ind
func TestSessionCookieWorksOverHTTP(t *testing.T) {
	app := newTestApp(t)
	cookie := loginCookie(t, app, "")

	if !strings.Contains(cookie, "user_session=") || !strings.Contains(cookie, "HttpOnly") {
		t.Fatalf("cookie %q should be set with HttpOnly", cookie)
	}
	if strings.Contains(cookie, "Secure") {
		t.Fatalf("cookie %q must not be Secure over http, the browser would not send it back", cookie)
	}
}

func TestSessionKeyComesFromTheEnvironment(t *testing.T) {
	t.Setenv("SESSION_KEY", "a-key-from-the-environment")
	if got := string(sessionKey()); got != "a-key-from-the-environment" {
		t.Fatalf("sessionKey() = %q", got)
	}
}

func TestRandomSessionKeyWithoutEnvironment(t *testing.T) {
	t.Setenv("SESSION_KEY", "")
	first, second := sessionKey(), sessionKey()
	if len(first) != 32 || string(first) == string(second) {
		t.Fatal("without SESSION_KEY each start should get a new random 32 byte key")
	}
}
