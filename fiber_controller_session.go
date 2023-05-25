package backend

// NewFiberSessionsCtrl creates a simple admin sessions controller for fiber.
func (backend *Backend) NewFiberSessionsCtrl() *fiberSessionsCtrl {
	return &fiberSessionsCtrl{
		backend: backend,
	}
}

type fiberSessionsCtrl struct {
	backend *Backend
}

func (ctrl fiberSessionsCtrl) Authenticate(c FiberCtx) error {
	user := ctrl.backend.FiberGetCurrentAdmin(c)
	if user == nil {
		c.SendStatus(401)
		return c.JSON(struct {
			Message string
		}{"Please Log In"})
	}
	return c.Next()
}

func (ctrl fiberSessionsCtrl) Me(c FiberCtx) error {
	admin := ctrl.backend.FiberGetCurrentAdmin(c)
	if a, ok := admin.(Serializable); ok {
		return c.JSON(a.Serialize("me"))
	}
	return c.JSON(admin)
}

func (ctrl fiberSessionsCtrl) SignIn(c FiberCtx) error {
	return c.JSON(struct {
		Token string
	}{ctrl.backend.MustFiberValidateNewSession(c)})
}

func (ctrl fiberSessionsCtrl) SignOut(c FiberCtx) error {
	ctrl.backend.MustFiberDeleteSession(c)
	return c.SendStatus(204)
}
