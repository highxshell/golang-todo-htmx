package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"

	"github.com/highxshell/golang-todo/services"
	"github.com/highxshell/golang-todo/templates"
	"github.com/highxshell/golang-todo/templates/components"
	"github.com/highxshell/golang-todo/templates/todo"
	echojwt "github.com/labstack/echo-jwt"
	"github.com/labstack/echo/v4"
)

func main() {
	userDB, err := sql.Open("sqlite3", "./db/chat.db")
	if err != nil {
		log.Println(err)
	}
	defer userDB.Close()
	sqlStmt := `
	create table if not exists user (id text not null primary key, username varchar(255), password varchar(255));
	`
	_, err = userDB.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}
	db, err := sql.Open("sqlite3", "./db/todo.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	sqlStmt2 := `
	create table if not exists todo (id text not null primary key, text text, checked bool);
	`
	_, err = db.Exec(sqlStmt2)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt2)
		return
	}
	userService := &services.UserService{
		DB: userDB,
	}
	todoService := &services.TodoService{
		DB: db,
	}
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unguardedRoutes := e.Group("/")
	unguardedRoutes.Use(services.GuestMiddleware)
	unguardedRoutes.GET("", func(c echo.Context) error {
		component := templates.Index()
		return component.Render(ctx, c.Response().Writer)
	})
	unguardedRoutes.GET("register", func(c echo.Context) error {
		component := templates.Register()
		return component.Render(ctx, c.Response().Writer)
	})
	guardedRoutes := e.Group("/todos")
	guardedRoutes.Use(services.TokenRefresherMiddleware)
	guardedRoutes.Use(echojwt.WithConfig(echojwt.Config{
		SigningKey:   []byte(services.JwtSecretKey),
		TokenLookup:  "cookie:access-token", // "<source>:<name>"
		ErrorHandler: services.JWTErrorChecker,
	}))
	guardedRoutes.GET("", func(c echo.Context) error {
		todos := todoService.GetTodos()
		component := todo.Index(todos)
		return component.Render(ctx, c.Response().Writer)
	})

	e.POST("/login", func(c echo.Context) error {
		username := c.FormValue("username")
		password := c.FormValue("password")
		loggedInUser, err := userService.LoginUser(username, password)
		if err != nil {
			fmt.Println(err)
			return echo.NewHTTPError(http.StatusUnauthorized, "Invalid login.")
		}

		// Assign JWT tokens
		err = services.GenerateTokensAndSetCookies(loggedInUser, c)

		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "Token failed to be generated.")
		}

		return c.Redirect(http.StatusMovedPermanently, "/todos")
	})

	e.POST("/register", func(c echo.Context) error {
		username := c.FormValue("username")
		password := c.FormValue("password")
		confirmPassword := c.FormValue("confirmPassword")

		// password validation
		if password != confirmPassword {
			return echo.NewHTTPError(http.StatusBadRequest, "Password is not the same as confirm password.")
		}

		// user validation
		users, err := userService.GetUsers(username)
		if err != nil || len(users) > 0 {
			fmt.Println(err)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid credentials.")
		}

		// create a new user
		newUser, err := userService.CreateUser(username, password)
		if err != nil {
			fmt.Println(err)
			return echo.NewHTTPError(http.StatusInternalServerError, "Invalid login.")
		}

		// Assign JWT tokens
		err = services.GenerateTokensAndSetCookies(newUser, c)

		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "Token failed to be generated.")
		}

		return c.Redirect(http.StatusMovedPermanently, "/")
	})

	e.POST("/todos", func(c echo.Context) error {
		text := c.FormValue("add-todo-input")
		if text == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid text")
		}
		todoService.CreateTodo(text)
		todos := todoService.GetTodos()
		component := components.TodoCardsWithBtn(todos)
		return component.Render(context.Background(), c.Response().Writer)
	})
	e.PUT("/todos/:id", func(c echo.Context) error {
		id := c.Param("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid id")
		}

		oldTodo := todoService.GetTodo(id)

		text := c.FormValue("edit-todo-input")
		if text == "" {
			text = oldTodo.Text
		}

		checkedString := c.FormValue("checked")
		var checked bool
		if checkedString == "on" {
			checked = true
		} else {
			checked = false
		}

		todo := todoService.UpdateTodo(id, text, checked)

		component := components.TodoCard(*todo)
		return component.Render(context.Background(), c.Response().Writer)
	})
	e.DELETE("/todos/:id", func(c echo.Context) error {
		id := c.Param("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid id")
		}
		todoService.DeleteTodo(id)
		todos := todoService.GetTodos()
		component := components.TodoCards(todos)
		return component.Render(context.Background(), c.Response().Writer)
	})
	e.GET("/components", func(c echo.Context) error {
		t := c.QueryParam("type")
		id := c.QueryParam("id")
		switch t {
		case "add-todo":
			component := components.AddTodoInput()
			return component.Render(context.Background(), c.Response().Writer)
		case "add-todo-btn":
			component := components.AddTodoButton()
			return component.Render(context.Background(), c.Response().Writer)
		case "edit-todo-input":
			todo := todoService.GetTodo(id)
			component := components.EditTodoInput(todo)
			return component.Render(context.Background(), c.Response().Writer)
		case "edit-todo-btn":
			todo := todoService.GetTodo(id)
			component := components.TodoCard(*todo)
			return component.Render(context.Background(), c.Response().Writer)
		}
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid element")
	})

	e.Static("/css", "css")
	e.Static("/static", "static")
	e.Static("/fonts", "fonts")
	e.Logger.Fatal(e.Start(":3000"))
}
