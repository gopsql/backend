package backend

import (
	"database/sql"
	"database/sql/driver"

	"github.com/go-playground/validator/v10"
	"github.com/gopsql/db"
	"github.com/gopsql/logger"
	"github.com/gopsql/psql"
)

type (
	// Backend instance.
	Backend struct {
		JWTSession jwtSession
		Validator  *validator.Validate

		models    []*psql.Model
		logger    logger.Logger
		errNoRows error
		toArray   toArray
	}

	// github.com/gopsql/jwt.Session
	jwtSession interface {
		GenerateAuthorization(userId int, sessionId string) (string, error)
		ParseAuthorization(auth string) (userId int, sessionId string, ok bool)
	}

	// github.com/lib/pq.Array
	toArray func(interface{}) interface {
		driver.Valuer
		sql.Scanner
	}
)

// Default backend contains Admin, AdminSession.
var Default = newDefaultBackend()

// Create new backend instance.
func NewBackend() *Backend {
	return &Backend{
		Validator: validator.New(),
		logger:    logger.NoopLogger,
	}
}

func newDefaultBackend() *Backend {
	b := NewBackend()
	b.NewModel(Admin{})
	b.NewModel(AdminSession{})
	b.Validator.RegisterValidation("uniqueness", func(fl validator.FieldLevel) bool {
		if i, ok := fl.Top().Interface().(interface{ IsUnique(*Backend, string) bool }); ok {
			return i.IsUnique(b, fl.StructFieldName())
		}
		return false
	})
	return b
}

// NewModel creates and returns a new psql.Model. See github.com/gopsql/psql.
func (backend *Backend) NewModel(object interface{}, options ...interface{}) *psql.Model {
	m := psql.NewModel(object, options...)
	backend.AddModels(m)
	return m
}

// AddModels adds one or multiple psql.Model instances to backend.
func (backend *Backend) AddModels(models ...*psql.Model) {
	backend.models = append(backend.models, models...)
}

// SetToArray sets github.com/lib/pq.Array function.
func (backend *Backend) SetToArray(f toArray) {
	backend.toArray = f
}

// SetConnection sets database connection.
func (backend *Backend) SetConnection(dbConn db.DB) {
	backend.errNoRows = dbConn.ErrNoRows()
	for _, m := range backend.models {
		m.SetConnection(dbConn)
	}
}

// SetLogger sets logger.
func (backend *Backend) SetLogger(logger logger.Logger) {
	backend.logger = logger
	for _, m := range backend.models {
		m.SetLogger(logger)
	}
}

// ModelByName finds psql.Model by name.
func (backend *Backend) ModelByName(name string) *psql.Model {
	for _, m := range backend.models {
		if m.TypeName() == name {
			return m
		}
	}
	return nil
}
