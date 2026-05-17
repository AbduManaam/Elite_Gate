package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "admin-API is Running",
		})
	})

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "9090" // Default port if not set
	}

	r.Run(fmt.Sprintf(":%s", port))
}
