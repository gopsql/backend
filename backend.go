package backend

import (
	"database/sql"
	"database/sql/driver"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gopsql/db"
	"github.com/gopsql/logger"
	"github.com/gopsql/migrator"
	"github.com/gopsql/psql"
)

type (
	// Backend instance.
	Backend struct {
		Name      string
		Validator *validator.Validate

		jwtSession jwtSession
		models     []*psql.Model
		logger     logger.Logger
		migrator   *migrator.Migrator
		errNoRows  error
		toArray    toArray
	}

	SQL       = psql.SQL
	SelectSQL = psql.SelectSQL
	InsertSQL = psql.InsertSQL
	DeleteSQL = psql.DeleteSQL
	UpdateSQL = psql.UpdateSQL

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
		migrator:  migrator.NewMigrator(),
	}
}

func newDefaultBackend() *Backend {
	b := NewBackend()
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

func (backend *Backend) AddModelAdmin() {
	backend.NewModel(Admin{})
}

func (backend *Backend) AddModelAdminSession() {
	backend.NewModel(AdminSession{})
}

// AddModels adds one or multiple psql.Model instances to backend.
func (backend *Backend) AddModels(models ...*psql.Model) {
	backend.models = append(backend.models, models...)
}

func (backend *Backend) SetName(name string) {
	backend.Name = name
}

// SetToArray sets github.com/lib/pq.Array function.
func (backend *Backend) SetToArray(f toArray) {
	backend.toArray = f
}

// SetConnection sets database connection.
func (backend *Backend) SetConnection(dbConn db.DB) {
	backend.errNoRows = dbConn.ErrNoRows()
	backend.migrator.SetConnection(dbConn)
	for _, m := range backend.models {
		m.SetConnection(dbConn)
	}
}

// SetLogger sets logger.
func (backend *Backend) SetLogger(logger logger.Logger) {
	backend.logger = logger
	backend.migrator.SetLogger(logger)
	for _, m := range backend.models {
		m.SetLogger(logger)
	}
}

func (backend *Backend) SetMigrations(migrations interface{}) {
	backend.migrator.SetMigrations(migrations)
}

func (backend *Backend) SetJWTSession(jwtSession jwtSession) {
	backend.jwtSession = jwtSession
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

type (
	flagUsages       [][]string
	flagUsageOptions []flagUsageOption
	flagUsageOption  int
)

const (
	NoCreateAdmin flagUsageOption = iota
	NoCreateConfig
	NoCreateMigration
	NoMigrate
	NoRollback
)

var allFlagUsages = flagUsages{
	{"CREATE_ADMIN=1", "reset first admin password or create new admin"},
	{"CREATE_CONFIG=1", "create new config or update existing config"},
	{"CREATE_MIGRATION=1", "generate new migration file"},
	{"MIGRATE=1", "run new migrations"},
	{"ROLLBACK=1", "rollback last migration"},
}

func (options flagUsageOptions) has(option flagUsageOption) bool {
	for _, o := range options {
		if o == option {
			return true
		}
	}
	return false
}

func (options flagUsageOptions) flagUsages() (out flagUsages) {
	for i, usage := range allFlagUsages {
		if options.has(flagUsageOption(i)) {
			continue
		}
		out = append(out, usage)
	}
	return
}

func (usages flagUsages) String() string {
	var max int
	for _, usage := range usages {
		if len(usage[0]) > max {
			max = len(usage[0])
		}
	}
	var lines []string
	for _, usage := range usages {
		spaces := strings.Repeat(" ", max-len(usage[0]))
		lines = append(lines, fmt.Sprintf("  %s%s - %s", usage[0], spaces, usage[1]))
	}
	return strings.Join(lines, "\n")
}

func (backend *Backend) FlagUsage(options ...flagUsageOption) func() {
	usage := flagUsageOptions(options).flagUsages().String()
	return func() {
		o := flag.CommandLine.Output()
		fmt.Fprintf(o, "Usage: %s [OPTIONS] [ENVVARS...]\n", backend.Name)
		fmt.Fprintln(o)
		fmt.Fprintln(o, "Options:")
		flag.PrintDefaults()
		if usage != "" {
			fmt.Fprintln(o)
			fmt.Fprintln(o, "Available ENVVARS (environment variables):")
			fmt.Fprintln(o, usage)
		}
	}
}

func SetEnvFromArgs() {
	for _, arg := range flag.Args() {
		idx := strings.Index(arg, "=")
		if idx > -1 {
			os.Setenv(arg[0:idx], arg[idx+1:])
		}
	}
}
