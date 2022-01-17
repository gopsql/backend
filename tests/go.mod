module github.com/gopsql/backend/tests

go 1.16

replace github.com/gopsql/backend => ../

require (
	github.com/gofiber/fiber/v2 v2.24.0
	github.com/gopsql/backend v0.0.0
	github.com/gopsql/jwt v1.0.0
	github.com/gopsql/logger v1.0.0
	github.com/gopsql/sqlite v1.0.1
)
