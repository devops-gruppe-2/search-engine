package main

import (

	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
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

	router.POST("/register", registerUser)

	router.Run(":8080")

}

type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

func registerUser(c *gin.Context) {
	// Extract HTML form values directly using Gin
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, APIResponse{
			Status:  "error",
			Message: "Username and password are required",
		})
		return
	}

	c.JSON(http.StatusCreated, APIResponse{
		Status:  "success",
		Message: "User registered successfully",
		User:    username,
	})
}