# backend

Easily integrate admin to your fiber application.

## Usage

### Set database connection

```go
import (
	"github.com/gopsql/backend"
	"github.com/gopsql/logger"
	"github.com/gopsql/pq"
)

conn := pq.MustOpen("postgres://localhost:5432/mydb?sslmode=disable")
backend.Default.SetConnection(conn)
backend.Default.SetLogger(logger.StandardLogger)
backend.Default.CheckAdmin()
```

### Set ErrorHandler

```go
app := fiber.New(fiber.Config{
	ErrorHandler: func(c *fiber.Ctx, err error) error {
		status, content := backend.Default.HandleError(err)
		return c.Status(status).JSON(content)
	},
})
```

### Set JWTSession

```go
import (
	"github.com/gopsql/goconf"
	"github.com/gopsql/jwt"
)

type Configs struct {
	AdminSession *jwt.Session
}

var configs Configs
goconf.Unmarshal([]byte(configFileContent), &c)
backend.Default.JWTSession = configs.AdminSession
```

### Set fiber routes:

```go
import (
	"github.com/gofiber/fiber/v2"
	"github.com/gopsql/backend"
)

type Ctx = fiber.Ctx

func Group(g fiber.Router) {
	sc := backend.Default.NewFiberSessionsCtrl()
	g.Post("/sign-in", convert(sc.SignIn))
	g.Get("/me", convert(sc.Me))
	// routes below need authentication
	g.Use(convert(sc.Authenticate))
	g.Post("/sign-out", convert(sc.SignOut))

	ac := backend.Default.NewFiberAdminsCtrl()
	g.Get("/admins", convert(ac.List))
	g.Get("/admins/:id", convert(ac.Show))
	g.Post("/admins", convert(ac.Create))
	g.Put("/admins/:id", convert(ac.Update))
	g.Delete("/admins/:id", convert(ac.Destroy))
	g.Post("/admins/:id", convert(ac.Restore))
}

func convert(f backend.FiberHandler) fiber.Handler {
	return func(c *Ctx) error {
		return f(c)
	}
}
```

### Others

```go
// new model:
var Orders = backend.Default.NewModel(Order{})

// validate:
backend.Default.MustValidateStruct(foobar)
```
