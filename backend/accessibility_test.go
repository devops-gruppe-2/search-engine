package main

// Tests af tilgængelighed (opgaven "Improve accessibility"). De tjekker de ting, Lighthouse
// og WAVE fandt: felter uden label, ingen <main> og fejlbeskeder, som skærmlæsere ikke læser op.

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var (
	inputTag     = regexp.MustCompile(`<input\b[^>]*>`)
	idAttribute  = regexp.MustCompile(`\bid="([^"]+)"`)
	typeHidden   = regexp.MustCompile(`\btype="hidden"`)
	ariaLabel    = regexp.MustCompile(`\baria-label="[^"]+"`)
	labelForAttr = regexp.MustCompile(`<label\b[^>]*\bfor="([^"]+)"`)
)

// Hvert synligt inputfelt skal have en <label for="..."> eller aria-label, så en skærmlæser
// kan sige, hvad feltet er til
func TestEveryInputHasALabel(t *testing.T) {
	app := newTestApp(t)
	for _, path := range []string{"/", "/login", "/register"} {
		page := app.do(http.MethodGet, path, nil, true)
		expectStatus(t, page, http.StatusOK)
		checkLabels(t, path, page.body)
	}

	// Siden til at skifte password kræver login
	app.register("alice", "password-1")
	page := app.do(http.MethodGet, "/change-password", nil, true)
	expectStatus(t, page, http.StatusOK)
	checkLabels(t, "/change-password", page.body)
}

func checkLabels(t *testing.T, path, html string) {
	t.Helper()
	labelled := map[string]bool{}
	for _, m := range labelForAttr.FindAllStringSubmatch(html, -1) {
		labelled[m[1]] = true
	}
	inputs := inputTag.FindAllString(html, -1)
	if len(inputs) == 0 {
		t.Fatalf("%s has no inputs", path)
	}
	for _, input := range inputs {
		if typeHidden.MatchString(input) || ariaLabel.MatchString(input) {
			continue
		}
		id := idAttribute.FindStringSubmatch(input)
		if id == nil || !labelled[id[1]] {
			t.Errorf("%s: input has no label: %s", path, input)
		}
	}
}

// Alle sider har sprog, <main> og en navigation, som skærmlæsere kan hoppe til
func TestPagesHaveLandmarksAndLanguage(t *testing.T) {
	app := newTestApp(t)
	for _, path := range []string{"/", "/login", "/register"} {
		page := app.do(http.MethodGet, path, nil, true)
		expectStatus(t, page, http.StatusOK)
		for _, want := range []string{`<html lang="en">`, "<main", "</main>", "<nav", "<header", "<footer"} {
			if !strings.Contains(page.body, want) {
				t.Errorf("%s is missing %s", path, want)
			}
		}
	}
}

// Fejlbeskeder har role="alert", så en skærmlæser læser dem op med det samme
func TestErrorsAreAnnouncedToScreenReaders(t *testing.T) {
	app := newTestApp(t)
	res := app.do(http.MethodPost, "/api/login", url.Values{"username": {"nobody"}, "password": {"wrong"}}, true)
	if !strings.Contains(res.body, `class="error" role="alert"`) {
		t.Fatalf("login error has no role=\"alert\": %s", res.body)
	}
}
