package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Search engine backend is running",
		})
	})

	router.GET("/api/search", func(c *gin.Context) {
		q := c.Query("q")

		if q == "" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"statusCode": 422,
				"message":    "q is required",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": []interface{}{},
		})
	})

	router.Run(":8084")
}
