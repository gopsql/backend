package backend

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gopsql/goconf"
)

// ValidateStruct validates struct with github.com/go-playground/validator/v10.
func (backend Backend) ValidateStruct(i interface{}) error {
	if backend.Validator == nil {
		return errors.New("no validator")
	}
	return backend.Validator.Struct(i)
}

// MustValidateStruct is like ValidateStruct but panics if validation fails.
func (backend Backend) MustValidateStruct(i interface{}) {
	if err := backend.ValidateStruct(i); err != nil {
		panic(err)
	}
}

func (backend Backend) IsErrNoRows(err error) bool {
	return backend.dbConn != nil && backend.dbConn.ErrNoRows() == err
}

// HandleError returns status code and error message given error.
func (backend Backend) HandleError(err error) (status int, json interface{}) {
	if backend.IsErrNoRows(err) {
		return 404, struct{ Message string }{"Not Found"}
	}
	if errs, ok := err.(InputErrors); ok {
		return 400, map[string]interface{}{"Errors": errs}
	}
	if err, ok := err.(validator.ValidationErrors); ok {
		var errs InputErrors
		for _, e := range err {
			errs = append(errs, InputError{
				FullName: e.Namespace(),
				Name:     e.Field(),
				Kind:     e.Kind().String(),
				Type:     e.Tag(),
				Param:    e.Param(),
			})
		}
		return 400, map[string]interface{}{"Errors": errs}
	}
	backend.logger.Error("Server Error:", err)
	return 500, struct{ Message string }{"Server Error"}
}

// Create new admin with adminName and adminPassword (random password if empty)
// or reset password of admin with adminName to adminPassword. If adminName is
// empty, only name of first admin in database is returned.
func (backend Backend) CreateAdmin(adminName, adminPassword string) (name, password string, updated bool) {
	m := backend.ModelByName("Admin").Quiet()
	m.Select("name").OrderBy("id ASC").QueryRow(&name)
	if adminName == "" {
		return
	}
	if adminPassword == "" {
		password = randomString(8)
	} else {
		password = adminPassword
	}
	if name == "" {
		name = adminName
		admin := NewAdmin(name, password)
		var conflict string
		if m.Connection() != nil && m.Connection().DriverName() == "sqlite" {
			conflict = m.ToColumnName("Name")
		} else {
			conflict = fmt.Sprintf("lower(%s)", m.ToColumnName("Name"))
		}
		m.Insert(m.Permit("Name", "Password").Filter(*admin)).OnConflict(conflict).
			DoUpdate(
				fmt.Sprintf("%s = NULL", m.ToColumnName("DeletedAt")),
				fmt.Sprintf("%[1]s = EXCLUDED.%[1]s", m.ToColumnName("Password")),
			).MustExecute()
	} else {
		admin := NewAdmin(name, password)
		admin.DeletedAt = nil
		m.Update(m.Permit("Password", "DeletedAt").Filter(*admin)).Where(
			fmt.Sprintf("%s = $1", m.ToColumnName("Name")), name,
		).MustExecute()
		updated = true
	}
	return
}

// CheckAdmin prints a warning if database contains no admins. If
// CREATE_ADMIN=1 environment variable is set, creates new admin or resets
// existing admin's password.
func (backend Backend) CheckAdmin() {
	if os.Getenv("CREATE_ADMIN") == "1" {
		name, password, updated := backend.CreateAdmin("admin", "")
		if updated {
			backend.logger.Info("Password of admin has been reset:")
			backend.logger.Info("  - name:", name)
			backend.logger.Info("  - password:", password)
		} else {
			backend.logger.Info("New admin has been created:")
			backend.logger.Info("  - name:", name)
			backend.logger.Info("  - password:", password)
		}
		os.Exit(0)
	} else {
		name, _, _ := backend.CreateAdmin("", "")
		if name == "" {
			backend.logger.Warning("Warning: You have no admins. Use CREATE_ADMIN=1 to create one.")
		}
	}
}

func randomString(n int) string {
	rand.Seed(time.Now().UnixNano())
	const letterBytes = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// CheckMigrations prints a warning if there are migrations not yet run. If
// CREATE_MIGRATION=1 environment variable is set, create new migrationn file.
// If MIGRATE=1 environment variable is set, executes the up SQL for all the
// migrations that have not yet been run. If ROLLBACK=1 environment variable
// is set, rollback last migration.
func (backend Backend) CheckMigrations() {
	if os.Getenv("CREATE_MIGRATION") == "1" {
		migrations, err := backend.MigratorNewMigration()
		if err != nil {
			backend.logger.Fatal(err)
		}
		dir := "migrations"
		if err := os.MkdirAll(dir, 0755); err != nil {
			backend.logger.Fatal(err)
		}
		for _, migration := range migrations {
			path := filepath.Join(dir, migration.FileName())
			err := ioutil.WriteFile(path, []byte(migration.String()), 0644)
			if err != nil {
				backend.logger.Fatal(err)
			}
			backend.logger.Info("written", path)
		}
		os.Exit(0)
	}

	if _, unmigrated := backend.migrator.Versions(); len(unmigrated) > 0 {
		backend.logger.Warning("Warning: You have", len(unmigrated), "pending migrations. Use MIGRATE=1 to run migrations.")
	}

	if os.Getenv("MIGRATE") == "1" {
		backend.migrator.Migrate()
		os.Exit(0)
	}

	if os.Getenv("ROLLBACK") == "1" {
		backend.migrator.Rollback()
		os.Exit(0)
	}
}

// ReadConfigs uses github.com/gopsql/goconf to read config file into target
// config struct.
func (backend Backend) ReadConfigs(configFile string, target interface{}) {
	toCreate := os.Getenv("CREATE_CONFIG") == "1"
	err := readFile(configFile, target)
	if toCreate {
		if err := writeFile(configFile, target); err != nil {
			backend.logger.Error(err)
			os.Exit(1)
		} else {
			backend.logger.Info("Config file written:", configFile)
			os.Exit(0)
		}
	} else if err != nil {
		backend.logger.Warning("Warning: Error reading config file:", err)
		backend.logger.Warning("Use CREATE_CONFIG=1 create or update config file.")
	}
}

type canSetDefaultValues interface {
	SetDefaultValues()
}

func readFile(file string, target interface{}) error {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	err = goconf.Unmarshal(content, target)
	if err == nil {
		if t, ok := target.(canSetDefaultValues); ok {
			t.SetDefaultValues()
		}
	}
	return err
}

func writeFile(file string, conf interface{}) error {
	content, err := goconf.Marshal(conf)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(file, ".go") {
		content = append([]byte("// vi: set filetype=go :\n"), content...)
	}
	return ioutil.WriteFile(file, content, 0600)
}
