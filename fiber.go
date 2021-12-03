package backend

import "strings"

type (
	// Interface for github.com/gofiber/fiber/v2.Ctx
	FiberCtx interface {
		Body() []byte
		BodyParser(out interface{}) error
		Get(key string, defaultValue ...string) string
		IP() string
		JSON(data interface{}) error
		Locals(key string, value ...interface{}) (val interface{})
		Next() (err error)
		Params(key string, defaultValue ...string) string
		Query(key string, defaultValue ...string) string
		QueryParser(out interface{}) error
		SendStatus(status int) error
	}

	FiberHandler func(FiberCtx) error
)

// FiberNewSession creates new session for adminId and returns a new JWT
// string.
func (backend Backend) FiberNewSession(c FiberCtx, adminId int) (token string, err error) {
	var sessionId string
	m := backend.ModelByName("AdminSession")
	err = m.Insert(
		"AdminId", adminId,
		"IpAddress", c.IP(),
		"UserAgent", c.Get("User-Agent"),
	).Returning("session_id").QueryRow(&sessionId)
	if err != nil {
		return
	}
	err = m.Delete().Where("id IN (SELECT id FROM admin_sessions WHERE admin_id = $1 "+
		"ORDER BY updated_at DESC OFFSET $2)", adminId, 10).Execute()
	if err != nil {
		return
	}
	return backend.jwtSession.GenerateAuthorization(adminId, sessionId)
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
	return backend.ModelByName("AdminSession").Delete().
		Where("admin_id = $1 AND session_id = $2", adminId, sessionId).Execute()
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
	var admin Admin
	err := backend.ModelByName("Admin").Find().Where("lower(name) = $1", strings.ToLower(req.Name)).Query(&admin)
	if err != nil || !admin.Password.Equal(req.Password) {
		panic(NewInputErrors("Password", "wrong"))
	}
	if admin.DeletedAt != nil {
		panic(NewInputErrors("Name", "deleted"))
	}
	return backend.MustFiberNewSession(c, admin.Id)
}

// FiberGetAdminAndSessionId returns the admin and session ID from the
// Authorization header of a fiber context.
func (backend Backend) FiberGetAdminAndSessionId(c FiberCtx) (adminId int, sessionId string, ok bool) {
	return backend.jwtSession.ParseAuthorization(c.Get("Authorization"))
}

// FiberGetCurrentAdmin finds admin in the database and updates the admin
// session if IP address or user-agent has been changed, given the
// Authorization header of a fiber context. The returned admin is then cached
// in the current request, so subsequent calls of this function will not cause
// new database queries.
func (backend Backend) FiberGetCurrentAdmin(c FiberCtx) *Admin {
	if admin, ok := c.Locals("CurrentAdmin").(*Admin); ok && admin != nil {
		return admin
	}
	adminId, sessionId, ok := backend.FiberGetAdminAndSessionId(c)
	if !ok {
		return nil
	}
	admins := backend.ModelByName("Admin").Quiet()
	adminSessions := backend.ModelByName("AdminSession").Quiet()
	var admin Admin
	err := admins.Find().Where("deleted_at IS NULL AND id = $1", adminId).Query(&admin)
	if err != nil {
		return nil
	}
	var adminSession AdminSession
	err = adminSessions.Find().Where("admin_id = $1 AND session_id = $2", adminId, sessionId).Query(&adminSession)
	if err != nil {
		return nil
	}
	changes := []interface{}{}
	if ip := c.IP(); adminSession.IpAddress != ip {
		changes = append(changes, "IpAddress", ip)
	}
	if ua := c.Get("User-Agent"); adminSession.UserAgent != ua {
		changes = append(changes, "UserAgent", ua)
	}
	if len(changes) > 0 {
		if adminSessions.Update(changes...).Where("id = $1", adminSession.Id).Execute() != nil {
			return nil
		}
	}
	c.Locals("CurrentAdmin", &admin)
	return &admin
}
