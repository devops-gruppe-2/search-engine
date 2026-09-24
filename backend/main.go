package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"strings"

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
	Message       string         `json:"message"`
	SearchResults []SearchResult `json:"data"`
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

	router.LoadHTMLFiles("templates/register.html")
	router.GET("/register", func(c *gin.Context) {
		c.HTML(http.StatusOK, "register.html", nil)
	})


	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Search engine backend is running",
		})
	})

	router.POST("/api/register", registerUser)
	router.GET("/api/search", searchForStringInDB)
	router.POST("/api/login", loginUser)

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
// CREATE ENDPOIINTS
//==============================================================================================================================================================================

func registerUser(c *gin.Context) {

	username := c.PostForm("username")
	password := c.PostForm("password")
	email := c.PostForm("email")

	if username == "" || password == "" || email == "" {
		c.JSON(http.StatusBadRequest, APIResponse{
			Status:  "error",
			Message: "Username, email, and password are required",
		})
		return
	}
	query := "INSERT INTO users (username, email, password) VALUES (?, ?, ?)"
	_, err := db.Exec(query, username, email, password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Status:  "error",
			Message: "Failed to register user",
		})
		return
	}
	c.JSON(http.StatusCreated, APIResponse{
		Status:  "success",
		Message: "User registered successfully",
		User:    username,
	})
}

// Vores login funktion


//==============================================================================================================================================================================
// READ ENDPOINTS
//==============================================================================================================================================================================
func loginUser(c *gin.Context) {
	//Vi henter username og password efter at have oprettet variablerne
	//username og password
	username := c.PostForm("username")
	password := c.PostForm("password")

	//Her kontrollerer vi, at brugeren har skrevet begge dele
	if username == "" || password == "" {
		//JSON-svar sendes tilbage til klienten
		//ttp.StatusBadRequest er HTTP-status 400
		c.JSON(http.StatusBadRequest, APIResponse{
			//Klienten får
			Status:  "error",
			Message: "Username and password are required",
		})
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
		c.JSON(http.StatusUnauthorized, APIResponse{
			//logsinforsøg gav en fejl
			Status: "error",
			//Beskeden skjuler, om det er userame eler password, der er forkert skrevet
			Message: "Invalid username or password",
		})
		//Her asluttes JSON-objektet
		return
	}

	if password != storedPassword {
		c.JSON(http.StatusUnauthorized, APIResponse{
			Status:  "error",
			Message: "Invalid username or password",
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Status:  "success",
		Message: "Login successful",
		User:    username,
	})
}


func searchForStringInDB(c *gin.Context) {
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
			Status:  "error",
			Message: "Failed to search in database",
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
				Status:  "error",
				Message: "Failed to scan search results",
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
// CREATE ENDPOIINTS
//==============================================================================================================================================================================

func registerUser(c *gin.Context) {

	username := c.PostForm("username")
	password := c.PostForm("password")
	email := c.PostForm("email")

	if username == "" || password == "" || email == "" {
		c.JSON(http.StatusBadRequest, APIUserCreatedResponse{
			Status:  "error",
			Message: "Username, email, and password are required",
		})
		return
	}
	query := "INSERT INTO users (username, email, password) VALUES (?, ?, ?)"
	_, err := db.Exec(query, username, email, password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIUserCreatedResponse{
			Status:  "error",
			Message: "Failed to register user",
		})
		return
	}
	c.JSON(http.StatusCreated, APIUserCreatedResponse{
		Status:  "success",
		Message: "User registered successfully",
		User:    username,
	})
}

