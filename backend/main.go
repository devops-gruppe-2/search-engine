package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

//==============================================================================================================================================================================
// GLOBAL VARIABLES AND OTHER DEFINITIONS
//==============================================================================================================================================================================

//go:embed schema.sqlite
var schemaSQL string
var db *sql.DB

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
	defer db.Close()

	fmt.Println("Database initialized successfully")

	// Gin startup

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

	router.LoadHTMLFiles(
		"templates/search.html",
		"templates/register.html",
		"templates/login.html",
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

	// Forsiden er søgesiden, ligesom i legacy
	router.GET("/", HTMLSearchFunctionData)

	router.POST("/api/register", registerUser)
	router.GET("/api/search", SearchAPI)
	router.POST("/api/login", loginUser)
	router.GET("/search", HTMLSearchFunctionData)
	// OpenAPI-spec'en siger GET, POST beholdes så eksisterende klienter ikke går i stykker
	router.GET("/api/logout", logoutUser)
	router.POST("/api/logout", logoutUser)

	router.Run(":8080")

}

//==============================================================================================================================================================================
// DATABASE
//==============================================================================================================================================================================

func initDB(dbPath string) (*sql.DB, error) {

	dsn := dbPath + "?_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	statements := strings.Split(schemaSQL, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("failed to execute statement: %w", err)
		}
	}

	return db, nil
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
	query := "INSERT INTO users (username, email, password) VALUES (?, ?, ?)"
	_, err := db.Exec(query, username, email, password)
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

	// En bruger i browseren sendes videre til login-siden
	if wantsHTML(c) {
		c.Redirect(http.StatusSeeOther, "/login")
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

	//Opretter variabel, som holder det password, vi skal finde i databasen
	var storedPassword string
	//Vi laver query her, som betyder: find passwordet for denne bruger, hvis brugernavnet passer
	query := "SELECT password FROM users WHERE username =?"
	err := db.QueryRow(query, username).Scan(&storedPassword)

	//Hvis der skulle opstå en fejl under databaseopslaget
	if err != nil {
		//HTTP sender 401 til brugeren
		loginFailed(http.StatusUnauthorized, "Invalid username or password")
		//Her asluttes JSON-objektet
		return
	}

	if password != storedPassword {
		loginFailed(http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// Passwordet er korrekt, så vi henter brugerens session
	session := sessions.Default(c)

	// Gem brugernavnet i sessionen
	session.Set("username", username)

	// Gem sessionen og send cookien tilbage til browseren
	if err := session.Save(); err != nil {
		loginFailed(http.StatusInternalServerError, "Failed to create session")
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
	query := "SELECT * FROM pages WHERE language = ? AND content LIKE ?"

	rows, err := db.Query(query, language, "%"+SearchParameter+"%")
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIUserSearchResponse{
			StatusCode: 422,
			Message:    "Failed to search in database",
		})
		return
	}
	defer rows.Close()

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

	query := "SELECT * FROM pages WHERE language = ? AND content LIKE ?"
	rows, err := db.Query(query, language, "%"+searchParameter+"%")
	if err != nil {
		c.HTML(http.StatusInternalServerError, "search.html", pageData(c, gin.H{
			"Query":         searchParameter,
			"SearchResults": []SearchResult{},
			"Error":         "Failed to search in database",
		}))
		return
	}
	defer rows.Close()

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
