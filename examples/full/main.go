// Command full shows gindocs with optional request/response types and
// bearer authentication on a small in-memory users API.
//
//	go run ./examples/full
//
// Then open http://localhost:8081/docs, click Authorize and use the token
// "demo-token" (or get it from POST /login with password "demo").
package main

import (
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/anikchand461/gindocs"
	"github.com/gin-gonic/gin"
)

type User struct {
	ID        int       `json:"id" example:"1"`
	Name      string    `json:"name" example:"Ada Lovelace"`
	Email     string    `json:"email" binding:"email" example:"ada@example.com"`
	Role      string    `json:"role" binding:"oneof=admin member" description:"Access level"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateUserRequest struct {
	Name  string `json:"name" binding:"required,min=2,max=50" example:"Ada Lovelace"`
	Email string `json:"email" binding:"required,email" example:"ada@example.com"`
	Role  string `json:"role" binding:"omitempty,oneof=admin member" description:"Defaults to member"`
}

type ListUsersQuery struct {
	Role  string `form:"role" binding:"omitempty,oneof=admin member" description:"Only users with this role"`
	Q     string `form:"q" description:"Case-insensitive name filter"`
	Limit int    `form:"limit" binding:"omitempty,min=1,max=100" example:"20"`
}

type UserURI struct {
	ID int `uri:"id" binding:"required,min=1" description:"User ID"`
}

type UserList struct {
	Items []User `json:"items"`
	Total int    `json:"total"`
}

// LoginForm is posted as application/x-www-form-urlencoded.
type LoginForm struct {
	Username string `form:"username" binding:"required" example:"ada"`
	Password string `form:"password" binding:"required" description:"Use demo"`
}

type Token struct {
	Token string `json:"token" example:"demo-token"`
}

// AvatarForm is posted as multipart/form-data because it has file fields.
type AvatarForm struct {
	Avatar  *multipart.FileHeader   `form:"avatar" binding:"required" description:"PNG or JPEG image"`
	Extras  []*multipart.FileHeader `form:"extras" description:"Optional extra images"`
	Caption string                  `form:"caption" binding:"max=80"`
}

type AvatarInfo struct {
	UserID   int      `json:"userId"`
	Filename string   `json:"filename"`
	Size     int64    `json:"size"`
	Extras   []string `json:"extras"`
	Caption  string   `json:"caption"`
}

type APIError struct {
	Error string `json:"error" example:"something went wrong"`
}

type store struct {
	mu     sync.Mutex
	users  []User
	nextID int
}

var db = &store{nextID: 1}

func main() {
	r := gin.Default()

	r.GET("/health", Health)
	r.POST("/login", Login)

	api := r.Group("/api/v1", RequireToken)
	api.GET("/users", ListUsers)
	api.POST("/users", CreateUser)
	api.GET("/users/:id", GetUser)
	api.DELETE("/users/:id", DeleteUser)
	api.POST("/users/:id/avatar", UploadAvatar)
	api.GET("/files/*path", GetFile) // undocumented on purpose: zero config

	docs := gindocs.New(r)
	docs.Title = "Users API"
	docs.Description = "A small in-memory users API. Authorize with the token `demo-token`."
	docs.BearerAuth()

	docs.Route("GET /health").Public()
	docs.Route("POST /login").
		Public().
		Summary("Log in").
		Form(LoginForm{}).
		Response(200, Token{}).
		Response(401, APIError{})
	docs.Route("POST /api/v1/users/:id/avatar").
		Path(UserURI{}).
		Form(AvatarForm{}).
		Response(200, AvatarInfo{}).
		Response(400, APIError{})
	docs.Route("GET /api/v1/users").
		Query(ListUsersQuery{}).
		Response(200, UserList{}).
		Response(401, APIError{})
	docs.Route("POST /api/v1/users").
		Summary("Create a user").
		Description("Creates a user. The email must be unique.").
		Body(CreateUserRequest{}).
		Response(201, User{}).
		Response(400, APIError{}).
		Response(409, APIError{})
	docs.Route("GET /api/v1/users/:id").
		Path(UserURI{}).
		Response(200, User{}).
		Response(404, APIError{})
	docs.Route("DELETE /api/v1/users/:id").
		Path(UserURI{}).
		Response(204, nil).
		Response(404, APIError{})

	docs.Serve("/docs")

	r.Run(":8081")
}

// RequireToken rejects requests without "Authorization: Bearer demo-token".
func RequireToken(c *gin.Context) {
	if c.GetHeader("Authorization") != "Bearer demo-token" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, APIError{Error: "missing or invalid token"})
		return
	}
	c.Next()
}

func Login(c *gin.Context) {
	var f LoginForm
	if err := c.ShouldBind(&f); err != nil {
		c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	if f.Password != "demo" {
		c.JSON(http.StatusUnauthorized, APIError{Error: "wrong username or password"})
		return
	}
	c.JSON(http.StatusOK, Token{Token: "demo-token"})
}

func UploadAvatar(c *gin.Context) {
	var uri UserURI
	var f AvatarForm
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	if err := c.ShouldBind(&f); err != nil {
		c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	info := AvatarInfo{UserID: uri.ID, Filename: f.Avatar.Filename, Size: f.Avatar.Size, Extras: []string{}, Caption: f.Caption}
	for _, x := range f.Extras {
		info.Extras = append(info.Extras, x.Filename)
	}
	c.JSON(http.StatusOK, info)
}

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func ListUsers(c *gin.Context) {
	var q ListUsersQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	items := []User{}
	for _, u := range db.users {
		if (q.Role == "" || u.Role == q.Role) && strings.Contains(strings.ToLower(u.Name), strings.ToLower(q.Q)) {
			items = append(items, u)
		}
	}
	total := len(items)
	if q.Limit > 0 && len(items) > q.Limit {
		items = items[:q.Limit]
	}
	c.JSON(http.StatusOK, UserList{Items: items, Total: total})
}

func CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, u := range db.users {
		if strings.EqualFold(u.Email, req.Email) {
			c.JSON(http.StatusConflict, APIError{Error: "email already in use"})
			return
		}
	}
	u := User{ID: db.nextID, Name: req.Name, Email: req.Email, Role: req.Role, CreatedAt: time.Now().UTC()}
	db.nextID++
	db.users = append(db.users, u)
	c.JSON(http.StatusCreated, u)
}

func GetUser(c *gin.Context) {
	var uri UserURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, u := range db.users {
		if u.ID == uri.ID {
			c.JSON(http.StatusOK, u)
			return
		}
	}
	c.JSON(http.StatusNotFound, APIError{Error: "user not found"})
}

func DeleteUser(c *gin.Context) {
	var uri UserURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	i := slices.IndexFunc(db.users, func(u User) bool { return u.ID == uri.ID })
	if i < 0 {
		c.JSON(http.StatusNotFound, APIError{Error: "user not found"})
		return
	}
	db.users = slices.Delete(db.users, i, i+1)
	c.Status(http.StatusNoContent)
}

func GetFile(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"path": c.Param("path")})
}
