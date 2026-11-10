package main

// Integrationstests: de starter hele appen (router, middleware, sessions og en rigtig
// SQLite-database) i en testserver og sender rigtige HTTP-requests til den.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// testApp er en kørende app med sin egen tomme database og en klient, der husker cookies
type testApp struct {
	t      *testing.T
	server *httptest.Server
	client *http.Client
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()

	var err error
	db, err = initDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	server := httptest.NewServer(setupRouter())
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{
		Jar: jar,
		// Følg ikke redirects automatisk, så testene kan se hvor brugeren bliver sendt hen
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	return &testApp{t: t, server: server, client: client}
}

// response er det, testene har brug for fra et svar
type response struct {
	status   int
	location string
	body     string
}

func (a *testApp) do(method, path string, form url.Values, browser bool) response {
	a.t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(a.t.Context(), method, a.server.URL+path, body)
	if err != nil {
		a.t.Fatalf("request: %v", err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if browser {
		req.Header.Set("Accept", "text/html")
	}

	res, err := a.client.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatalf("read body: %v", err)
	}
	return response{status: res.StatusCode, location: res.Header.Get("Location"), body: string(data)}
}

func (a *testApp) get(path string) response { return a.do(http.MethodGet, path, nil, false) }

func (a *testApp) post(path string, form url.Values) response {
	return a.do(http.MethodPost, path, form, false)
}

func (a *testApp) register(username, password string) {
	a.t.Helper()
	res := a.post("/api/register", url.Values{
		"username": {username},
		"email":    {username + "@example.com"},
		"password": {password},
	})
	if res.status != http.StatusOK {
		a.t.Fatalf("register %s: status %d, body %s", username, res.status, res.body)
	}
}

func expectStatus(t *testing.T, res response, want int) {
	t.Helper()
	if res.status != want {
		t.Fatalf("status = %d, want %d (body: %s)", res.status, want, res.body)
	}
}

//==============================================================================================================================================================================
// REGISTER OG LOGIN
//==============================================================================================================================================================================

func TestRegisterThenLogin(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "correct-horse")

	res := app.post("/api/login", url.Values{"username": {"alice"}, "password": {"correct-horse"}})
	expectStatus(t, res, http.StatusOK)
}

// User feedback survey: brugeren skal være logget ind direkte efter registrering
func TestRegisterLogsTheUserIn(t *testing.T) {
	app := newTestApp(t)

	form := url.Values{"username": {"alice"}, "email": {"alice@example.com"}, "password": {"correct-horse"}}
	res := app.do(http.MethodPost, "/api/register", form, true)
	expectStatus(t, res, http.StatusSeeOther)
	if res.location != "/" {
		t.Fatalf("register redirected to %q, want /", res.location)
	}

	page := app.do(http.MethodGet, "/", nil, true)
	if !strings.Contains(page.body, `id="nav-logout"`) || !strings.Contains(page.body, "alice") {
		t.Fatal("front page should show the logout link with the username right after register")
	}

	// Sessionen er rigtig: logout virker uden at logge ind først
	expectStatus(t, app.get("/api/logout"), http.StatusOK)
}

func TestRegisterThroughTheAPIAlsoLogsTheUserIn(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "correct-horse")

	expectStatus(t, app.get("/api/logout"), http.StatusOK)
}

func TestPasswordsAreStoredAsBcryptHashes(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "correct-horse")

	var stored string
	if err := db.QueryRowContext(t.Context(), "SELECT password FROM users WHERE username = 'alice'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "correct-horse" || !strings.HasPrefix(stored, "$2") {
		t.Fatalf("password is not stored as a bcrypt hash: %q", stored)
	}
}

func TestLoginRejectsWrongPasswordAndUnknownUser(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "correct-horse")

	expectStatus(t, app.post("/api/login", url.Values{"username": {"alice"}, "password": {"wrong"}}), http.StatusUnauthorized)
	expectStatus(t, app.post("/api/login", url.Values{"username": {"nobody"}, "password": {"x"}}), http.StatusUnauthorized)
}

func TestLoginRequiresUsernameAndPassword(t *testing.T) {
	app := newTestApp(t)

	tests := []struct {
		name string
		form url.Values
	}{
		{"both missing", url.Values{}},
		{"username missing", url.Values{"password": {"x"}}},
		{"password missing", url.Values{"username": {"alice"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectStatus(t, app.post("/api/login", tt.form), http.StatusUnprocessableEntity)
		})
	}
}

// registerProblems sender en registrering og giver de felter, API'et siger er ugyldige (loc og type)
func registerProblems(t *testing.T, app *testApp, form url.Values) []string {
	t.Helper()
	res := app.post("/api/register", form)
	expectStatus(t, res, http.StatusUnprocessableEntity)

	var body HTTPValidationError
	if err := json.Unmarshal([]byte(res.body), &body); err != nil {
		t.Fatalf("422 body is not HTTPValidationError: %v (%s)", err, res.body)
	}
	var problems []string
	for _, d := range body.Detail {
		if len(d.Loc) != 2 || d.Loc[0] != "body" || d.Msg == "" {
			t.Fatalf("invalid validation error: %+v", d)
		}
		problems = append(problems, d.Loc[1]+":"+d.Type)
	}
	return problems
}

func expectProblems(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("problems = %v, want %v", got, want)
	}
}

// OpenAPI: 200 med AuthResponse {"statusCode", "message"}
func TestRegisterRespondsLikeTheSpec(t *testing.T) {
	app := newTestApp(t)
	res := app.post("/api/register", url.Values{"username": {"alice"}, "email": {"alice@example.com"}, "password": {"correct-horse"}})
	expectStatus(t, res, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal([]byte(res.body), &body); err != nil {
		t.Fatal(err)
	}
	if body["statusCode"] != float64(200) || body["message"] != "User registered successfully" || len(body) != 2 {
		t.Fatalf("body = %v, want AuthResponse with statusCode 200", body)
	}
}

func TestRegisterRequiresAllFields(t *testing.T) {
	app := newTestApp(t)
	expectProblems(t, registerProblems(t, app, url.Values{}), "username:missing", "email:missing", "password:missing")
	expectProblems(t, registerProblems(t, app, url.Values{"username": {"alice"}, "password": {"x"}}), "email:missing")
}

func TestRegisterRejectsInvalidEmail(t *testing.T) {
	app := newTestApp(t)
	form := url.Values{"username": {"alice"}, "email": {"not-an-email"}, "password": {"correct-horse"}}
	expectProblems(t, registerProblems(t, app, form), "email:value_error")
}

func TestRegisterChecksPassword2WhenItIsSent(t *testing.T) {
	app := newTestApp(t)
	form := url.Values{"username": {"alice"}, "email": {"alice@example.com"}, "password": {"correct-horse"}, "password2": {"other"}}
	expectProblems(t, registerProblems(t, app, form), "password2:value_error")

	form.Set("password2", "correct-horse")
	expectStatus(t, app.post("/api/register", form), http.StatusOK)
}

func TestRegisterRejectsDuplicateUsernameAndEmail(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "correct-horse")

	sameName := url.Values{"username": {"alice"}, "email": {"other@example.com"}, "password": {"correct-horse"}}
	expectProblems(t, registerProblems(t, app, sameName), "username:value_error")

	sameEmail := url.Values{"username": {"bob"}, "email": {"alice@example.com"}, "password": {"correct-horse"}}
	expectProblems(t, registerProblems(t, app, sameEmail), "email:value_error")
}

func TestRegisterFormShowsTheError(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "correct-horse")

	form := url.Values{"username": {"alice"}, "email": {"other@example.com"}, "password": {"correct-horse"}}
	page := app.do(http.MethodPost, "/api/register", form, true)
	expectStatus(t, page, http.StatusUnprocessableEntity)
	if !strings.Contains(page.body, "The username is already taken") || !strings.Contains(page.body, `value="alice"`) {
		t.Fatal("register page should show the error and keep what the user typed")
	}
}

// Mange brugere, der registrerer sig samtidig, må ikke få "database is locked" (fejl 500)
func TestManyUsersCanRegisterAtTheSameTime(t *testing.T) {
	app := newTestApp(t)

	const users = 20
	statuses := make(chan int, users)
	var wg sync.WaitGroup
	for i := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			form := url.Values{
				"username": {fmt.Sprintf("user%d", i)},
				"email":    {fmt.Sprintf("user%d@example.com", i)},
				"password": {"correct-horse"},
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.server.URL+"/api/register", strings.NewReader(form.Encode()))
			if err != nil {
				statuses <- 0
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				statuses <- 0
				return
			}
			_ = res.Body.Close()
			statuses <- res.StatusCode
		}()
	}
	wg.Wait()
	close(statuses)

	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("a registration got status %d, want %d", status, http.StatusOK)
		}
	}
}

//==============================================================================================================================================================================
// SESSION OG LOGOUT
//==============================================================================================================================================================================

func TestBrowserLoginShowsLogoutLinkUntilLogout(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "correct-horse")

	res := app.do(http.MethodPost, "/api/login", url.Values{"username": {"alice"}, "password": {"correct-horse"}}, true)
	expectStatus(t, res, http.StatusSeeOther)
	if res.location != "/" {
		t.Fatalf("login redirected to %q, want /", res.location)
	}

	page := app.do(http.MethodGet, "/", nil, true)
	if !strings.Contains(page.body, `id="nav-logout"`) || !strings.Contains(page.body, "alice") {
		t.Fatal("front page should show the logout link with the username after login")
	}

	expectStatus(t, app.get("/api/logout"), http.StatusOK)

	page = app.do(http.MethodGet, "/", nil, true)
	if !strings.Contains(page.body, `id="nav-login"`) {
		t.Fatal("front page should show the login link after logout")
	}

	// Når man er logget ud, er der ingen session at logge ud af
	expectStatus(t, app.get("/api/logout"), http.StatusUnauthorized)
}

//==============================================================================================================================================================================
// SØGNING
//==============================================================================================================================================================================

func TestSearchReturnsMatchingPagesInTheChosenLanguage(t *testing.T) {
	app := newTestApp(t)

	pages := []struct{ title, url, language, content string }{
		{"Go", "https://en.wikipedia.org/wiki/Go", "en", "Go is a programming language designed at Google."},
		{"Python", "https://en.wikipedia.org/wiki/Python", "en", "Python is a high-level programming language."},
		{"Go (dansk)", "https://da.wikipedia.org/wiki/Go", "da", "Go er et programmeringssprog fra Google."},
	}
	for _, p := range pages {
		_, err := db.ExecContext(t.Context(), "INSERT INTO pages (title, url, language, content) VALUES (?, ?, ?, ?)", p.title, p.url, p.language, p.content)
		if err != nil {
			t.Fatal(err)
		}
	}

	titles := func(path string) []string {
		res := app.get(path)
		expectStatus(t, res, http.StatusOK)
		var body APIUserSearchResponse
		if err := json.Unmarshal([]byte(res.body), &body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		var got []string
		for _, r := range body.SearchResults {
			got = append(got, r.Title)
		}
		return got
	}

	if got := titles("/api/search?q=Google"); len(got) != 1 || got[0] != "Go" {
		t.Fatalf("english search for Google = %v, want [Go]", got)
	}
	if got := titles("/api/search?q=Google&language=da"); len(got) != 1 || got[0] != "Go (dansk)" {
		t.Fatalf("danish search for Google = %v, want [Go (dansk)]", got)
	}
	if got := titles("/api/search?q=nothing-matches-this"); len(got) != 0 {
		t.Fatalf("search without matches = %v, want no results", got)
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	app := newTestApp(t)
	expectStatus(t, app.get("/api/search"), http.StatusBadRequest)
}

//==============================================================================================================================================================================
// SECURITY BREACH
//==============================================================================================================================================================================

func TestBreachedUserMustChangePassword(t *testing.T) {
	app := newTestApp(t)

	// En bruger fra før bcrypt med password i klartekst
	if _, err := db.ExecContext(t.Context(), "INSERT INTO users (username, email, password) VALUES ('bob', 'bob@example.com', 'old-password')"); err != nil {
		t.Fatal(err)
	}
	if err := migrateUsers(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	login := url.Values{"username": {"bob"}, "password": {"old-password"}}
	expectStatus(t, app.post("/api/login", login), http.StatusForbidden)

	// Resten af systemet er blokeret, indtil passwordet er skiftet
	expectStatus(t, app.get("/api/search?q=x"), http.StatusForbidden)

	invalid := []struct {
		name      string
		password  string
		password2 string
	}{
		{"too short", "short", "short"},
		{"not matching", "new-password-1", "new-password-2"},
		{"same as old", "old-password", "old-password"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			res := app.post("/api/change-password", url.Values{"password": {tt.password}, "password2": {tt.password2}})
			expectStatus(t, res, http.StatusUnprocessableEntity)
		})
	}

	res := app.post("/api/change-password", url.Values{"password": {"new-password-1"}, "password2": {"new-password-1"}})
	expectStatus(t, res, http.StatusOK)

	expectStatus(t, app.get("/api/search?q=x"), http.StatusOK)
	expectStatus(t, app.post("/api/login", login), http.StatusUnauthorized)
	expectStatus(t, app.post("/api/login", url.Values{"username": {"bob"}, "password": {"new-password-1"}}), http.StatusOK)
}

func TestChangePasswordRequiresLogin(t *testing.T) {
	app := newTestApp(t)
	res := app.post("/api/change-password", url.Values{"password": {"new-password-1"}, "password2": {"new-password-1"}})
	expectStatus(t, res, http.StatusUnauthorized)
}

//==============================================================================================================================================================================
// FRONTEND-KONTRAKT MED SIMULATOREN
//==============================================================================================================================================================================

// Simulatoren klikker rundt på siden med de selectors, der står i commence_the_rewrite.md
func TestPagesHaveTheSelectorsTheSimulatorUses(t *testing.T) {
	app := newTestApp(t)

	required := map[string][]string{
		"/":      {`id="search-input"`, `id="search-button"`, `id="nav-login"`},
		"/login": {`name="username"`, `name="password"`, `id="login-button"`},
	}
	for path, selectors := range required {
		page := app.do(http.MethodGet, path, nil, true)
		expectStatus(t, page, http.StatusOK)
		for _, selector := range selectors {
			if !strings.Contains(page.body, selector) {
				t.Errorf("%s is missing %s", path, selector)
			}
		}
	}
}
