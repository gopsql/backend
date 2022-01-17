package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gopsql/backend"
	"github.com/gopsql/jwt"
	"github.com/gopsql/logger"
	"github.com/gopsql/sqlite"
)

var app *fiber.App

func init() {
	backend.Default.AddModelAdmin()
	backend.Default.AddModelAdminSession()

	var l logger.Logger
	if os.Getenv("DEBUG") == "1" {
		l = logger.StandardLogger
	} else {
		l = logger.NoopLogger
	}
	backend.Default.SetLogger(l)

	backend.Default.SetJWTSession(jwt.NewSession(&jwt.SessionOptions{
		UserIdKeyName:    "AdminId",
		SessionIdKeyName: "SessionId",
	}))

	app = fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			status, content := backend.Default.HandleError(err)
			return c.Status(status).JSON(content)
		},
	})
	app.Use(recover.New())

	sc := backend.Default.NewFiberSessionsCtrl()
	app.Post("/sign-in", wrap(sc.SignIn))
	app.Get("/me", wrap(sc.Me))
	// routes below need authentication
	app.Use(wrap(sc.Authenticate))
	app.Post("/sign-out", wrap(sc.SignOut))

	ac := backend.Default.NewFiberAdminsCtrl()
	app.Get("/admins", wrap(ac.List))
	app.Get("/admins/:id", wrap(ac.Show))
	app.Post("/admins", wrap(ac.Create))
	app.Put("/admins/:id", wrap(ac.Update))
	app.Delete("/admins/:id", wrap(ac.Destroy))
	app.Post("/admins/:id", wrap(ac.Restore))
}

func wrap(f backend.FiberHandler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return f(c)
	}
}

func testWithSqlite(test func()) {
	const sqliteFile = "test.sqlite3"
	defer os.Remove(sqliteFile)
	conn := sqlite.MustOpen(sqliteFile)
	defer conn.Close()
	backend.Default.SetConnection(conn)
	backend.Default.SetMigrations(nil)
	migrations, err := backend.Default.MigratorNewMigration()
	if err != nil {
		panic(err)
	}
	backend.Default.SetMigrations(migrations)
	backend.Default.Migrator().Migrate()
	test()
}

func asJson(v interface{}) io.Reader {
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(v)
	return &buf
}

type test struct{ *testing.T }

type tokenResponse struct {
	Token string
}

func (t *test) Request(req *http.Request, expectedStatus int, v interface{}, extras ...interface{}) {
	if req.Method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, extra := range extras {
		switch e := extra.(type) {
		case tokenResponse:
			req.Header.Set("Authorization", e.Token)
		}
	}
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Error(err)
	}
	t.Int(fmt.Sprintf("%s %s status code", req.Method, req.URL.RequestURI()), resp.StatusCode, expectedStatus)
	defer resp.Body.Close()
	if v == nil {
		return
	}
	err = json.NewDecoder(resp.Body).Decode(v)
	if err != nil {
		t.Error(err)
	}
}

func (t *test) Bool(name string, got, expected bool) {
	t.Helper()
	if got == expected {
		t.Logf("%s test passed", name)
	} else {
		t.Errorf("%s test failed, expected %t, got %t", name, expected, got)
	}
}

func (t *test) String(name, got, expected string) {
	t.Helper()
	if got == expected {
		t.Logf("%s test passed", name)
	} else {
		t.Errorf("%s test failed, expected %s, got %s", name, expected, got)
	}
}

func (t *test) Int(name string, got, expected int) {
	t.Helper()
	if got == expected {
		t.Logf("%s test passed", name)
	} else {
		t.Errorf("%s test failed, expected %d, got %d", name, expected, got)
	}
}
