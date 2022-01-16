package backend

import (
	"fmt"
	"strings"
	"time"

	"github.com/gopsql/bcrypt"
	"github.com/gopsql/psql"
)

type (
	// Simple admin with name and password.
	Admin struct {
		Id        int
		Name      string          `validate:"gt=0,lte=30,uniqueness"`
		Password  bcrypt.Password `validate:"required"`
		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt *time.Time
	}

	// Admin session contains session ID, IP address and user-agent.
	AdminSession struct {
		Id        int
		AdminId   int
		SessionId string
		IpAddress string
		UserAgent string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	// Used in admins controller.
	SerializerAdmin struct {
		Id        int
		Name      string
		Password  *string
		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt *time.Time

		SessionsCount *int
	}

	// Used in admin sessions controller.
	SerializerAdminSimple struct {
		Id   int
		Name string
	}
)

func (Admin) AfterCreateSchema(m psql.Model) string {
	if m.Connection().DriverName() == "sqlite" {
		return fmt.Sprintf("CREATE UNIQUE INDEX unique_admin ON %s (%s COLLATE NOCASE);",
			m.TableName(), m.ToColumnName("Name"))
	}
	return fmt.Sprintf("CREATE UNIQUE INDEX unique_admin ON %s USING btree (lower(%s));",
		m.TableName(), m.ToColumnName("Name"))
}

func (Admin) DataType(m psql.Model, fieldName string) (dataType string) {
	if fieldName == "DeletedAt" {
		if m.Connection() != nil && m.Connection().DriverName() == "sqlite" {
			dataType = "timestamp"
		} else {
			dataType = "timestamptz"
		}
	}
	return
}

func (a Admin) IsUnique(backend *Backend, field string) bool { // uniqueness
	if field == "Name" {
		return !backend.ModelByName("Admin").
			Where("lower(name) = $1 AND id != $2", strings.ToLower(a.Name), a.Id).MustExists()
	}
	return true
}

func (AdminSession) AfterCreateSchema(m psql.Model) string {
	return fmt.Sprintf("CREATE UNIQUE INDEX unique_admin_session ON %s (%s, %s);",
		m.TableName(), m.ToColumnName("AdminId"), m.ToColumnName("SessionId"))
}

func (AdminSession) DataType(m psql.Model, fieldName string) (dataType string) {
	if fieldName == "SessionId" {
		if m.Connection() != nil && m.Connection().DriverName() == "sqlite" {
			dataType = "text NOT NULL DEFAULT (hex(randomblob(16)))"
		} else {
			dataType = "UUID NOT NULL DEFAULT gen_random_uuid()"
		}
	}
	return
}

// NewAdmin creates new admin with name and password.
func NewAdmin(name, password string) *Admin {
	admin := &Admin{
		Name: name,
	}
	admin.Password.MustUpdate(password)
	return admin
}

// NewSerializerAdmin converts Admin to SerializerAdmin.
func NewSerializerAdmin(a *Admin) *SerializerAdmin {
	if a == nil {
		return nil
	}
	return &SerializerAdmin{
		Id:        a.Id,
		Name:      a.Name,
		Password:  nil,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
		DeletedAt: a.DeletedAt,

		SessionsCount: nil,
	}
}

// NewSerializerAdminSimple converts Admin to SerializerAdminSimple.
func NewSerializerAdminSimple(a *Admin) *SerializerAdminSimple {
	if a == nil {
		return nil
	}
	return &SerializerAdminSimple{
		Id:   a.Id,
		Name: a.Name,
	}
}
