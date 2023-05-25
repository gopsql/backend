package backend

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gopsql/goconf"
)

// ValidateStruct validates struct or slice of structs with the validator
// (github.com/go-playground/validator/v10). If slice of struct is provided,
// each struct will be validated and nil or ValidatorFieldErrors is returned.
// ValidatorFieldErrors contains the indexes of the erroneous structs.
func (backend Backend) ValidateStruct(i interface{}) error {
	if backend.Validator == nil {
		return errors.New("no validator")
	}
	if reflect.TypeOf(i).Kind() == reflect.Slice {
		var verrs ValidatorFieldErrors
		rv := reflect.ValueOf(i)
		for i := 0; i < rv.Len(); i++ {
			err := backend.Validator.Struct(rv.Index(i).Interface())
			if err == nil {
				continue
			}
			if errs, ok := err.(validator.ValidationErrors); ok {
				for _, err := range errs {
					verrs = append(verrs, ValidatorFieldError{err, i})
				}
			}
		}
		if len(verrs) == 0 {
			return nil
		}
		return verrs
	}
	return backend.Validator.Struct(i)
}

// MustValidateStruct is like ValidateStruct but panics if validation fails.
func (backend Backend) MustValidateStruct(i interface{}) {
	if err := backend.ValidateStruct(i); err != nil {
		panic(err)
	}
}

type (
	// ValidatorFieldErrors can be returned when validating a slice of
	// struct using ValidateStruct().
	ValidatorFieldErrors []ValidatorFieldError

	// ValidatorFieldError contains the FieldError of the validator
	// (github.com/go-playground/validator/v10) and index of the struct in
	// the slice.
	ValidatorFieldError struct {
		FieldError validator.FieldError
		Index      int
	}
)

func (errs ValidatorFieldErrors) Error() string {
	buf := bytes.NewBufferString("")
	for i := 0; i < len(errs); i++ {
		buf.WriteString(errs[i].Error())
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func (v ValidatorFieldError) Error() string {
	return v.FieldError.Error()
}

func validatorFieldErrorToInputError(e validator.FieldError) InputError {
	return InputError{
		FullName: e.Namespace(),
		Name:     e.Field(),
		Kind:     e.Kind().String(),
		Type:     e.Tag(),
		Param:    e.Param(),
	}
}

// Check if given error equals to ErrNoRows() of the database connection.
func (backend Backend) IsErrNoRows(err error) bool {
	return backend.dbConn != nil && backend.dbConn.ErrNoRows() == err
}

// HandleError returns status code and error message struct according to the
// given error.
func (backend Backend) HandleError(err error) (status int, json interface{}) {
	if backend.IsErrNoRows(err) {
		return 404, struct{ Message string }{"Not Found"}
	}
	switch errs := err.(type) {
	case InputErrors:
		return 400, map[string]interface{}{"Errors": errs}
	case validator.ValidationErrors:
		var ierrs InputErrors
		for _, e := range errs {
			ierrs = append(ierrs, validatorFieldErrorToInputError(e))
		}
		return 400, map[string]interface{}{"Errors": ierrs}
	case ValidatorFieldErrors:
		var ierrs []InputErrorWithIndex
		for _, e := range errs {
			ierrs = append(ierrs, InputErrorWithIndex{validatorFieldErrorToInputError(e.FieldError), e.Index})
		}
		return 400, map[string]interface{}{"Errors": ierrs}
	}
	backend.logger.Error("Server Error:", err)
	return 500, struct{ Message string }{"Server Error"}
}

// Create new admin with adminName and adminPassword (random password if empty)
// or reset password of admin with adminName to adminPassword. If adminName is
// empty, only name of first admin in database is returned.
func (backend Backend) CreateAdmin(adminName, adminPassword string) (name, password string, updated bool) {
	m := backend.ModelByName("Admin").Quiet()
	var admin IsAdmin
	if a, ok := m.New().Interface().(IsAdmin); ok {
		admin = a
	} else {
		backend.logger.Fatal("no admin model")
	}
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
		admin.SetName(name)
		admin.SetPassword(password)
		var conflict string
		if m.Connection() != nil && m.Connection().DriverName() == "sqlite" {
			conflict = m.ToColumnName("Name")
		} else {
			conflict = fmt.Sprintf("lower(%s)", m.ToColumnName("Name"))
		}
		m.Insert("Name", admin.GetName(), "Password", admin.GetPassword()).OnConflict(conflict).
			DoUpdate(
				fmt.Sprintf("%s = NULL", m.ToColumnName("DeletedAt")),
				fmt.Sprintf("%[1]s = EXCLUDED.%[1]s", m.ToColumnName("Password")),
			).MustExecute()
	} else {
		admin.SetPassword(password)
		m.Update("Password", admin.GetPassword(), "DeletedAt", nil).WHERE("Name", "=", name).MustExecute()
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
			var action, content string
			var path string = filepath.Join(dir, migration.FileName(-1))
			if _, err := os.Stat(path); err == nil { // overwrite existing
				content = migration.String(-1)
				action = "overwritten"
			} else { // create new
				path = filepath.Join(dir, migration.FileName())
				content = migration.String()
				action = "created"
			}
			err := ioutil.WriteFile(path, []byte(content), 0644)
			if err != nil {
				backend.logger.Fatal(err)
			}
			backend.logger.Info(action, path)
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
// config struct. This will create or update config file if environment
// variable CREATE_CONFIG is set to 1.
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

// ReadConfigFile uses github.com/gopsql/goconf to read config file into target
// config struct.
func (backend Backend) ReadConfigFile(configFile string, target interface{}) error {
	return readFile(configFile, target)
}

// WriteConfigFile uses github.com/gopsql/goconf to write config file with
// target config struct.
func (backend Backend) WriteConfigFile(configFile string, target interface{}) error {
	return writeFile(configFile, target)
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
