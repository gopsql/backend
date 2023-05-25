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

	IsAdmin interface {
		GetId() int
		GetName() string
		GetPassword() bcrypt.Password
		GetCreatedAt() time.Time
		GetUpdatedAt() time.Time
		GetDeletedAt() *time.Time
		SetId(int)
		SetName(string)
		SetPassword(string) error
		SetCreatedAt(time.Time)
		SetUpdatedAt(time.Time)
		SetDeletedAt(*time.Time)
	}

	Serializable interface {
		Serialize(typ string, data ...interface{}) interface{}
	}
)

var (
	_ IsAdmin = (*Admin)(nil)
)

func (a Admin) GetId() int                         { return a.Id }
func (a Admin) GetName() string                    { return a.Name }
func (a Admin) GetPassword() bcrypt.Password       { return a.Password }
func (a Admin) GetCreatedAt() time.Time            { return a.CreatedAt }
func (a Admin) GetUpdatedAt() time.Time            { return a.UpdatedAt }
func (a Admin) GetDeletedAt() *time.Time           { return a.DeletedAt }
func (a *Admin) SetId(id int)                      { a.Id = id }
func (a *Admin) SetName(name string)               { a.Name = name }
func (a *Admin) SetPassword(password string) error { return a.Password.Update(password) }
func (a *Admin) SetCreatedAt(createdAt time.Time)  { a.CreatedAt = createdAt }
func (a *Admin) SetUpdatedAt(updatedAt time.Time)  { a.UpdatedAt = updatedAt }
func (a *Admin) SetDeletedAt(deletedAt *time.Time) { a.DeletedAt = deletedAt }

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

var (
	_ Serializable = (*Admin)(nil)
)

type (
	adminForMe struct {
		Id   int
		Name string
	}
)

func (a Admin) Serialize(typ string, data ...interface{}) interface{} {
	switch typ {
	case "me":
		return adminForMe{
			Id:   a.Id,
			Name: a.Name,
		}
	}
	return a
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
