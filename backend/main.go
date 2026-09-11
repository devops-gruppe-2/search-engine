package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	_ "embed"
)

//go:embed schema.sqlite
var schemaSQL string
var db *sql.DB

func main() {

	var err error
	db, err = initDB("./app.db")

	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	fmt.Println("Database initialized successfully")


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

	router.Run(":8080")

}

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

type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

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

