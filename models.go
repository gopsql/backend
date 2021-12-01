package backend

import (
	"strings"
	"time"

	"github.com/gopsql/bcrypt"
)

type (
	// Simple admin with name and password.
	Admin struct {
		Id        int
		Name      string          `validate:"gt=0,lte=30,uniqueness"`
		Password  bcrypt.Password `validate:"required"`
		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt *time.Time `dataType:"timestamptz"`
	}

	// Admin session contains session ID, IP address and user-agent.
	AdminSession struct {
		Id        int
		AdminId   int
		SessionId string `dataType:"UUID NOT NULL DEFAULT gen_random_uuid()"`
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

func (Admin) AfterCreateSchema() string {
	return `CREATE UNIQUE INDEX unique_admin ON admins USING btree (lower(name));`
}

func (a Admin) IsUnique(backend *Backend, field string) bool { // uniqueness
	if field == "Name" {
		return !backend.ModelByName("Admin").
			Where("lower(name) = $1 AND id != $2", strings.ToLower(a.Name), a.Id).MustExists()
	}
	return true
}

func (AdminSession) AfterCreateSchema() string {
	return `CREATE UNIQUE INDEX unique_admin_session ON admin_sessions (admin_id, session_id);`
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
