package backend

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gopsql/bcrypt"
)

type (
	// Interface for github.com/gofiber/fiber/v2.Ctx
	FiberCtx interface {
		Body() []byte
		BodyParser(out interface{}) error
		Get(key string, defaultValue ...string) string
		IP() string
		JSON(data interface{}) error
		Locals(key interface{}, value ...interface{}) (val interface{})
		Next() (err error)
		Params(key string, defaultValue ...string) string
		Query(key string, defaultValue ...string) string
		QueryParser(out interface{}) error
		SendStatus(status int) error
	}

	FiberHandler func(FiberCtx) error
)

func getName(c FiberCtx, defaultName string) string {
	if value, ok := c.Locals("Name" + defaultName).(string); ok && value != "" {
		return value
	}
	return defaultName
}

// FiberNewSession creates new session for adminId and returns a new JWT
// string.
func (backend Backend) FiberNewSession(c FiberCtx, adminId int) (token string, err error) {
	var sessionId string
	m := backend.ModelByName(getName(c, "AdminSession"))
	err = m.Insert(
		getName(c, "AdminId"), adminId,
		"IpAddress", c.IP(),
		"UserAgent", c.Get("User-Agent"),
	).Returning(m.ToColumnName(getName(c, "SessionId"))).QueryRow(&sessionId)
	if err != nil {
		return
	}
	var limit string
	if m.Connection() != nil && m.Connection().DriverName() == "sqlite" {
		limit = "LIMIT -1" // SQLite must have LIMIT clause
	}
	sql := fmt.Sprintf("%[1]s IN (SELECT %[1]s FROM %s WHERE %s = $1 ORDER BY %s DESC %s OFFSET $2)",
		m.ToColumnName("Id"), m.TableName(), m.ToColumnName(getName(c, "AdminId")), m.ToColumnName("UpdatedAt"), limit)
	err = m.Delete().Where(sql, adminId, 10).Execute()
	if err != nil {
		return
	}
	return backend.jwtSession.GenerateAuthorization(strconv.Itoa(adminId), sessionId)
}

// MustFiberNewSession is like FiberNewSession but panics if session creations
// fails.
func (backend Backend) MustFiberNewSession(c FiberCtx, adminId int) string {
	token, err := backend.FiberNewSession(c, adminId)
	if err != nil {
		panic(err)
	}
	return token
}

// FiberDeleteSession deletes a session in the database.
func (backend Backend) FiberDeleteSession(c FiberCtx) error {
	adminId, sessionId, _ := backend.FiberGetAdminAndSessionId(c)
	return backend.ModelByName(getName(c, "AdminSession")).Delete().
		WHERE(getName(c, "AdminId"), "=", adminId, getName(c, "SessionId"), "=", sessionId).Execute()
}

// MustFiberDeleteSession is like FiberDeleteSession but panics if session
// deletion fails.
func (backend Backend) MustFiberDeleteSession(c FiberCtx) {
	if err := backend.FiberDeleteSession(c); err != nil {
		panic(err)
	}
}

func (backend Backend) MustFiberValidateNewSession(c FiberCtx) string {
	var req struct {
		Name     string `validate:"gt=0,lte=30"`
		Password string `validate:"gte=6,lte=72"`
	}
	c.BodyParser(&req)
	backend.MustValidateStruct(req)
	var id int
	var password bcrypt.Password
	var deletedAt *time.Time
	m := backend.ModelByName(getName(c, "Admin"))
	err := m.Select("Id", "Password", "DeletedAt").
		Where(fmt.Sprintf("lower(%s) = $1", m.ToColumnName("Name")), strings.ToLower(req.Name)).
		QueryRow(&id, &password, &deletedAt)
	if err != nil || !password.Equal(req.Password) {
		panic(NewInputErrors("Password", "wrong"))
	}
	if deletedAt != nil {
		panic(NewInputErrors("Name", "deleted"))
	}
	return backend.MustFiberNewSession(c, id)
}

// FiberGetAdminAndSessionId returns the admin and session ID from the
// Authorization header of a fiber context.
func (backend Backend) FiberGetAdminAndSessionId(c FiberCtx) (adminId int, sessionId string, ok bool) {
	id, sessionId, ok := backend.jwtSession.ParseAuthorization(c.Get("Authorization"))
	if adminId, err := strconv.Atoi(id); err == nil {
		return adminId, sessionId, ok
	}
	return
}

// FiberGetCurrentAdmin finds admin in the database and updates the admin
// session if IP address or user-agent has been changed, given the
// Authorization header of a fiber context. The returned admin is then cached
// in the current request, so subsequent calls of this function will not cause
// new database queries.
func (backend Backend) FiberGetCurrentAdmin(c FiberCtx) interface{} {
	if admin := c.Locals(getName(c, "CurrentAdmin")); admin != nil {
		return admin
	}
	adminId, sessionId, ok := backend.FiberGetAdminAndSessionId(c)
	if !ok {
		return nil
	}
	admins := backend.ModelByName(getName(c, "Admin")).Quiet()
	adminSessions := backend.ModelByName(getName(c, "AdminSession")).Quiet()
	admin := admins.New().Interface()
	err := admins.Find().Where(fmt.Sprintf("%s IS NULL AND %s = $1",
		admins.ToColumnName("DeletedAt"), admins.ToColumnName("Id")), adminId).Query(admin)
	if err != nil {
		return nil
	}
	adminSession := adminSessions.New().Interface()
	err = adminSessions.Find().WHERE(getName(c, "AdminId"), "=", adminId, getName(c, "SessionId"), "=", sessionId).Query(adminSession)
	if err != nil {
		return nil
	}
	if as, ok := adminSession.(IsAdminSession); ok {
		changes := []interface{}{}
		if ip := c.IP(); as.GetIpAddress() != ip {
			changes = append(changes, "IpAddress", ip)
		}
		if ua := c.Get("User-Agent"); as.GetUserAgent() != ua {
			changes = append(changes, "UserAgent", ua)
		}
		if len(changes) > 0 {
			if adminSessions.Update(changes...).WHERE("Id", "=", as.GetId()).Execute() != nil {
				return nil
			}
		}
	}
	c.Locals(getName(c, "CurrentAdmin"), admin)
	return admin
}
