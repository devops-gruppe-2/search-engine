package main

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

//==============================================================================================================================================================================
// GLOBAL VARIABLES AND OTHER DEFINITIONS
//==============================================================================================================================================================================

//go:embed schema.sqlite
var schemaSQL string
var db *sql.DB

// Søgning i pages. Kolonnerne nævnes i samme rækkefølge som felterne i SearchResult,
// og en side uden dato giver en tom tekst i stedet for at få Scan til at fejle.
const searchQuery = "SELECT title, url, language, COALESCE(last_updated, ''), content FROM pages WHERE language = ? AND content LIKE ?"

// Krav til nye passwords
const minPasswordLength = 8

// bcrypt bruger kun de første 72 bytes af et password
const maxPasswordBytes = 72

type APIUserCreatedResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Language    string `json:"language"`
	LastUpdated string `json:"last_updated"`
	Content     string `json:"content"`
}

type APIUserSearchResponse struct {
	Status        string         `json:"status"`
	StatusCode    int            `json:"status_code,omitempty"`
	Message       string         `json:"message"`
	SearchResults []SearchResult `json:"data"`
}

type AuthResponse struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

//==============================================================================================================================================================================
// MAIN
//==============================================================================================================================================================================

func main() {

	// Database startup
	var err error
	db, err = initDB("./app.db")

	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	fmt.Println("Database initialized successfully")

	router := setupRouter()

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

// setupRouter samler alle routes og middleware, så de samme kan bruges af tests
func setupRouter() *gin.Engine {
	router := gin.Default()

	// session håndtering middleware
	store := cookie.NewStore(
		[]byte("0123456789abcdef0123456789abcdef"),
	)

	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	router.Use(sessions.Sessions("user_session", store))
	router.Use(requirePasswordChange)

	router.LoadHTMLFiles(
		"templates/search.html",
		"templates/register.html",
		"templates/login.html",
		"templates/change_password.html",
		"templates/weather.html",
		"templates/header.html",
	)

	// CSS og andre statiske filer
	router.Static("/static", "./static")

	router.GET("/register", func(c *gin.Context) {
		if isLoggedIn(c) {
			c.Redirect(http.StatusSeeOther, "/")
			return
		}
		c.HTML(http.StatusOK, "register.html", pageData(c, gin.H{}))
	})

	router.GET("/login", func(c *gin.Context) {
		if isLoggedIn(c) {
			c.Redirect(http.StatusSeeOther, "/")
			return
		}
		c.HTML(http.StatusOK, "login.html", pageData(c, gin.H{}))
	})

	// Brugere ramt af sikkerhedsbruddet skal vælge et nyt password her
	router.GET("/change-password", func(c *gin.Context) {
		if !isLoggedIn(c) {
			c.Redirect(http.StatusSeeOther, "/login")
			return
		}
		c.HTML(http.StatusOK, "change_password.html", pageData(c, gin.H{"MustChange": mustChangePassword(c)}))
	})

	// Forsiden er søgesiden, ligesom i legacy
	router.GET("/", HTMLSearchFunctionData)

	router.POST("/api/register", registerUser)
	router.GET("/api/search", SearchAPI)
	router.POST("/api/login", loginUser)
	router.POST("/api/change-password", changePassword)
	router.GET("/search", HTMLSearchFunctionData)
	router.GET("/weather", WeatherPage)
	router.GET("/api/weather", WeatherAPI)
	// OpenAPI-spec'en siger GET, POST beholdes så eksisterende klienter ikke går i stykker
	router.GET("/api/logout", logoutUser)
	router.POST("/api/logout", logoutUser)

	return router
}

//==============================================================================================================================================================================
// DATABASE
//==============================================================================================================================================================================

func initDB(dbPath string) (*sql.DB, error) {

	// busy_timeout: når en anden request skriver til databasen, venter vi op til 5 sekunder
	// på låsen i stedet for at fejle med det samme ("database is locked")
	dsn := dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Opstarten har ingen request, så vi bruger en baggrunds-context
	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	statements := strings.Split(schemaSQL, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("failed to execute statement: %w", err)
		}
	}

	if err := migrateUsers(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to migrate users: %w", err)
	}

	return db, nil
}

// migrateUsers gør en database fra før bcrypt klar. Den er sikker at køre ved hver opstart,
// fordi den kun rører brugere der stadig har et password i klartekst.
func migrateUsers(ctx context.Context, db *sql.DB) error {
	var columns int
	query := "SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'must_change_password'"
	if err := db.QueryRowContext(ctx, query).Scan(&columns); err != nil {
		return err
	}
	if columns == 0 {
		alter := "ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0"
		if _, err := db.ExecContext(ctx, alter); err != nil {
			return err
		}
	}

	// bcrypt-hashes starter altid med $2, så alt andet er klartekst
	rows, err := db.QueryContext(ctx, "SELECT id, password FROM users WHERE password NOT LIKE '$2%'")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type plainUser struct {
		id       int
		password string
	}
	var users []plainUser
	for rows.Next() {
		var user plainUser
		if err := rows.Scan(&user.id, &user.password); err != nil {
			return err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Klartekst-passwords må anses som stjålet (Security Breach), så brugerne skal også vælge et nyt
	update := "UPDATE users SET password = ?, must_change_password = 1 WHERE id = ?"
	for _, user := range users {
		hash, err := hashPassword(user.password)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, update, hash, user.id); err != nil {
			return err
		}
	}

	if len(users) > 0 {
		log.Printf("Hashed passwords for %d users", len(users))
	}
	return nil
}

// hashPassword laver et bcrypt-hash. bcrypt afviser passwords over 72 bytes,
// så de bliver forkortet her, både når de gemmes og når de tjekkes.
func hashPassword(password string) (string, error) {
	if len(password) > maxPasswordBytes {
		password = password[:maxPasswordBytes]
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// checkPassword tjekker et password mod et gemt bcrypt-hash
func checkPassword(hash, password string) bool {
	if len(password) > maxPasswordBytes {
		password = password[:maxPasswordBytes]
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

//==============================================================================================================================================================================
// HTML HELPERS
//==============================================================================================================================================================================

// wantsHTML er true når kaldet kommer fra en browser (fx en formular), så vi kan svare med en side i stedet for JSON
func wantsHTML(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "text/html")
}

// isLoggedIn tjekker om der er en bruger gemt i sessionen
func isLoggedIn(c *gin.Context) bool {
	return sessions.Default(c).Get("username") != nil
}

// mustChangePassword tjekker om den indloggede bruger skal vælge et nyt password
func mustChangePassword(c *gin.Context) bool {
	return sessions.Default(c).Get("must_change_password") == true
}

// requirePasswordChange sender brugere, der skal skifte password, til skifte-siden,
// så de ikke kan bruge resten af systemet først
func requirePasswordChange(c *gin.Context) {
	path := c.Request.URL.Path
	allowed := path == "/change-password" || path == "/api/change-password" ||
		path == "/api/logout" || strings.HasPrefix(path, "/static/")

	if allowed || !mustChangePassword(c) {
		c.Next()
		return
	}

	if wantsHTML(c) {
		c.Redirect(http.StatusSeeOther, "/change-password")
	} else {
		c.JSON(http.StatusForbidden, AuthResponse{
			StatusCode: http.StatusForbidden,
			Message:    "Password must be changed",
		})
	}
	c.Abort()
}

// pageData tilføjer login-status, som headeren bruger til at vise log ind/log ud
func pageData(c *gin.Context, data gin.H) gin.H {
	if username, ok := sessions.Default(c).Get("username").(string); ok {
		data["LoggedIn"] = true
		data["Username"] = username
	}
	return data
}

//==============================================================================================================================================================================
// CREATE ENDPOIINTS
//==============================================================================================================================================================================

func registerUser(c *gin.Context) {

	username := c.PostForm("username")
	password := c.PostForm("password")
	email := c.PostForm("email")

	// Bruges når formularen skal vises igen med en fejl
	formData := gin.H{"FormUsername": username, "FormEmail": email}

	if username == "" || password == "" || email == "" {
		if wantsHTML(c) {
			formData["Error"] = "Username, email, and password are required"
			c.HTML(http.StatusBadRequest, "register.html", pageData(c, formData))
			return
		}
		c.JSON(http.StatusBadRequest, APIUserCreatedResponse{
			Status:  "error",
			Message: "Username, email, and password are required",
		})
		return
	}
	hash, err := hashPassword(password)
	if err != nil {
		if wantsHTML(c) {
			formData["Error"] = "Failed to register user"
			c.HTML(http.StatusInternalServerError, "register.html", pageData(c, formData))
			return
		}
		c.JSON(http.StatusInternalServerError, APIUserCreatedResponse{
			Status:  "error",
			Message: "Failed to register user",
		})
		return
	}

	query := "INSERT INTO users (username, email, password) VALUES (?, ?, ?)"
	_, err = db.ExecContext(c.Request.Context(), query, username, email, hash)
	if err != nil {
		if wantsHTML(c) {
			formData["Error"] = "Failed to register user"
			c.HTML(http.StatusInternalServerError, "register.html", pageData(c, formData))
			return
		}
		c.JSON(http.StatusInternalServerError, APIUserCreatedResponse{
			Status:  "error",
			Message: "Failed to register user",
		})
		return
	}

	// Brugeren logges ind med det samme, så de ikke skal skrive det hele igen på login-siden
	// (User feedback survey). Nye brugere har et bcrypt-hash og skal ikke skifte password.
	session := sessions.Default(c)
	session.Set("username", username)
	session.Set("must_change_password", false)
	if err := session.Save(); err != nil {
		// Brugeren er oprettet, så de kan stadig logge ind selv
		log.Printf("Failed to log in new user %s: %v", username, err)
		if wantsHTML(c) {
			c.Redirect(http.StatusSeeOther, "/login")
			return
		}
	} else if wantsHTML(c) {
		// En bruger i browseren sendes til forsiden, hvor log ud-linket nu vises
		c.Redirect(http.StatusSeeOther, "/")
		return
	}

	c.JSON(http.StatusCreated, APIUserCreatedResponse{
		Status:  "success",
		Message: "User registered successfully",
		User:    username,
	})
}

//==============================================================================================================================================================================
// READ ENDPOINTS
//==============================================================================================================================================================================

// Vores login funktion
func loginUser(c *gin.Context) {
	//Vi henter username og password efter at have oprettet variablerne
	//username og password
	username := c.PostForm("username")
	password := c.PostForm("password")

	// Browseren får login-siden med en fejlbesked, alle andre får JSON
	loginFailed := func(status int, message string) {
		if wantsHTML(c) {
			c.HTML(status, "login.html", pageData(c, gin.H{"Error": message, "FormUsername": username}))
			return
		}
		c.JSON(status, AuthResponse{StatusCode: status, Message: message})
	}

	//Her kontrollerer vi, at brugeren har skrevet begge dele
	if username == "" || password == "" {
		//JSON-svar sendes tilbage til klienten
		//ttp.StatusBadRequest er HTTP-status 422
		loginFailed(http.StatusUnprocessableEntity, "Username and password are required")
		return
	}

	//Opretter variabler, som holder det password-hash, vi skal finde i databasen,
	//og om brugeren skal skifte password
	var storedHash string
	var mustChange bool
	//Vi laver query her, som betyder: find hashet for denne bruger, hvis brugernavnet passer
	query := "SELECT password, must_change_password FROM users WHERE username =?"
	err := db.QueryRowContext(c.Request.Context(), query, username).Scan(&storedHash, &mustChange)

	//Hvis der skulle opstå en fejl under databaseopslaget
	if err != nil {
		//HTTP sender 401 til brugeren
		loginFailed(http.StatusUnauthorized, "Invalid username or password")
		//Her asluttes JSON-objektet
		return
	}

	if !checkPassword(storedHash, password) {
		loginFailed(http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// Passwordet er korrekt, så vi henter brugerens session
	session := sessions.Default(c)

	// Gem brugernavnet i sessionen
	session.Set("username", username)
	session.Set("must_change_password", mustChange)

	// Gem sessionen og send cookien tilbage til browseren
	if err := session.Save(); err != nil {
		loginFailed(http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Password fra før bcrypt: brugeren skal vælge et nyt, før noget andet virker
	if mustChange {
		if wantsHTML(c) {
			c.Redirect(http.StatusSeeOther, "/change-password")
			return
		}
		c.JSON(http.StatusForbidden, AuthResponse{
			StatusCode: http.StatusForbidden,
			Message:    "Password must be changed",
		})
		return
	}

	// En bruger i browseren sendes til forsiden, hvor log ud-linket nu vises
	if wantsHTML(c) {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	c.JSON(http.StatusOK, AuthResponse{
		StatusCode: http.StatusOK,
		Message:    "Login successful",
	})
}

// changePassword gemmer et nyt password for den indloggede bruger
func changePassword(c *gin.Context) {
	session := sessions.Default(c)
	username, ok := session.Get("username").(string)
	if !ok {
		if wantsHTML(c) {
			c.Redirect(http.StatusSeeOther, "/login")
			return
		}
		c.JSON(http.StatusUnauthorized, AuthResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "You are not logged in",
		})
		return
	}

	password := c.PostForm("password")
	password2 := c.PostForm("password2")

	// Browseren får siden igen med en fejlbesked, alle andre får JSON
	changeFailed := func(status int, message string) {
		if wantsHTML(c) {
			c.HTML(status, "change_password.html", pageData(c, gin.H{
				"MustChange": mustChangePassword(c),
				"Error":      message,
			}))
			return
		}
		c.JSON(status, AuthResponse{StatusCode: status, Message: message})
	}

	if len(password) < minPasswordLength {
		changeFailed(http.StatusUnprocessableEntity, fmt.Sprintf("Password must be at least %d characters", minPasswordLength))
		return
	}
	if password != password2 {
		changeFailed(http.StatusUnprocessableEntity, "The two passwords do not match")
		return
	}

	// Det gamle password må anses som stjålet, så det må ikke bruges igen
	var storedHash string
	query := "SELECT password FROM users WHERE username = ?"
	if err := db.QueryRowContext(c.Request.Context(), query, username).Scan(&storedHash); err != nil {
		changeFailed(http.StatusInternalServerError, "Failed to change password")
		return
	}
	if checkPassword(storedHash, password) {
		changeFailed(http.StatusUnprocessableEntity, "The new password must be different from the old one")
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		changeFailed(http.StatusInternalServerError, "Failed to change password")
		return
	}

	update := "UPDATE users SET password = ?, must_change_password = 0 WHERE username = ?"
	if _, err := db.ExecContext(c.Request.Context(), update, hash, username); err != nil {
		changeFailed(http.StatusInternalServerError, "Failed to change password")
		return
	}

	session.Delete("must_change_password")
	if err := session.Save(); err != nil {
		changeFailed(http.StatusInternalServerError, "Failed to change password")
		return
	}

	if wantsHTML(c) {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	c.JSON(http.StatusOK, AuthResponse{
		StatusCode: http.StatusOK,
		Message:    "Password changed",
	})
}

func SearchAPI(c *gin.Context) {
	SearchParameter := c.Query("q")
	if SearchParameter == "" {
		c.JSON(http.StatusBadRequest, APIUserSearchResponse{
			Status:  "error",
			Message: "Search query is required",
		})
		return
	}
	language := c.DefaultQuery("language", "en")
	query := searchQuery

	rows, err := db.QueryContext(c.Request.Context(), query, language, "%"+SearchParameter+"%")
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIUserSearchResponse{
			StatusCode: 422,
			Message:    "Failed to search in database",
		})
		return
	}
	defer func() { _ = rows.Close() }()

	var results []SearchResult

	for rows.Next() {
		var result SearchResult
		err := rows.Scan(&result.Title, &result.URL, &result.Language, &result.LastUpdated, &result.Content)
		if err != nil {
			c.JSON(http.StatusInternalServerError, APIUserSearchResponse{
				StatusCode: 422,
				Message:    "Failed to scan search results",
			})
			return
		}
		results = append(results, result)
	}

	c.JSON(http.StatusOK, APIUserSearchResponse{
		Status:        "success",
		Message:       "Search results found",
		SearchResults: results,
	})

}

func HTMLSearchFunctionData(c *gin.Context) {
	searchParameter := c.Query("q")
	language := c.DefaultQuery("language", "en")

	if searchParameter == "" {
		c.HTML(http.StatusOK, "search.html", pageData(c, gin.H{
			"Query":         "",
			"SearchResults": []SearchResult{},
		}))
		return
	}

	query := searchQuery
	rows, err := db.QueryContext(c.Request.Context(), query, language, "%"+searchParameter+"%")
	if err != nil {
		c.HTML(http.StatusInternalServerError, "search.html", pageData(c, gin.H{
			"Query":         searchParameter,
			"SearchResults": []SearchResult{},
			"Error":         "Failed to search in database",
		}))
		return
	}
	defer func() { _ = rows.Close() }()

	var results []SearchResult

	for rows.Next() {
		var result SearchResult
		err := rows.Scan(&result.Title, &result.URL, &result.Language, &result.LastUpdated, &result.Content)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "search.html", pageData(c, gin.H{
				"Query":         searchParameter,
				"SearchResults": []SearchResult{},
				"Error":         "Failed to scan search results",
			}))
			return
		}
		results = append(results, result)
	}

	var errorMessage string
	if len(results) == 0 {
		errorMessage = "No results found"
	}

	c.HTML(http.StatusOK, "search.html", pageData(c, gin.H{
		"Query":         searchParameter,
		"SearchResults": results,
		"Error":         errorMessage,
	}))

}

func logoutUser(c *gin.Context) {

	session := sessions.Default(c)

	username := session.Get("username")

	if username == nil {
		if wantsHTML(c) {
			c.Redirect(http.StatusSeeOther, "/")
			return
		}
		c.JSON(http.StatusUnauthorized, AuthResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "You are not logged in",
		})
		return
	}

	session.Clear()

	session.Options(sessions.Options{
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, AuthResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to log out",
		})
		return
	}

	if wantsHTML(c) {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	c.JSON(http.StatusOK, AuthResponse{
		StatusCode: http.StatusOK,
		Message:    "Logout successful",
	})

}

//==============================================================================================================================================================================
// READ ENDPOINTS
//==============================================================================================================================================================================
