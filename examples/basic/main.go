// Command basic shows gin-swagger-ui on a small Gin API.
//
//	go run ./examples/basic
//
// Then open http://localhost:8080/docs.
package main

import (
	"net/http"

	gindocs "github.com/anikchand461/gin-swagger-ui"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.GET("/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "users"})
	})
	r.POST("/users", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"message": "user created"})
	})
	r.GET("/users/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})

	gindocs.New(r).Serve("/docs")

	r.Run(":8080")
}
