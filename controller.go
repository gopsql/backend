package backend

import (
	"errors"
	"math/rand"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
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

// HandleError returns status code and error message given error.
func (backend Backend) HandleError(err error) (status int, json interface{}) {
	if err == backend.errNoRows {
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

// CheckAdmin prints a warning if database contains no admins. If
// CREATE_ADMIN=1 environment variable is set, creates new admin or resets
// existing admin's password.
func (backend Backend) CheckAdmin() {
	m := backend.ModelByName("Admin").Quiet()

	var name string
	m.Select("name").OrderBy("id ASC").QueryRow(&name)

	if os.Getenv("CREATE_ADMIN") == "1" {
		password := randomString(8)
		if name == "" {
			name = "admin"
			admin := NewAdmin(name, password)
			m.Insert(m.Permit("Name", "Password").Filter(*admin)).OnConflict("lower(name)").
				DoUpdate("deleted_at = NULL", "password = EXCLUDED.password").MustExecute()
			backend.logger.Info("New admin has been created:")
			backend.logger.Info("  - name:", name)
			backend.logger.Info("  - password:", password)
		} else {
			admin := NewAdmin(name, password)
			admin.DeletedAt = nil
			m.Update(m.Permit("Password", "DeletedAt").Filter(*admin)).Where("name = $1", name).MustExecute()
			backend.logger.Info("Password of admin has been reset:")
			backend.logger.Info("  - name:", name)
			backend.logger.Info("  - password:", password)
		}
		os.Exit(0)
	} else {
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
