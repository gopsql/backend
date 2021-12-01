package backend

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gopsql/pagination/v2"
)

// NewFiberAdminsCtrl creates a simple admins controller for fiber.
func (backend *Backend) NewFiberAdminsCtrl() *fiberAdminsCtrl {
	return &fiberAdminsCtrl{
		backend: backend,
	}
}

type fiberAdminsCtrl struct {
	backend *Backend
}

func (ctrl fiberAdminsCtrl) List(c FiberCtx) error {
	q := pagination.PaginationQuerySort{
		pagination.Pagination{
			MaxPer:     50,
			DefaultPer: 20,
		},
		pagination.Query{},
		pagination.Sort{
			AllowedSorts: []string{
				"name",
				"created_at",
			},
			DefaultSort:  "created_at",
			DefaultOrder: "asc",
		},
	}
	pagination.Bind(&q, c.QueryParser)

	var cond []string
	var args []interface{}
	if pattern := q.GetLikePattern(); pattern != "" {
		cond = append(cond, "name ILIKE $?")
		args = append(args, pattern)
	}
	if c.Query("status") == "deleted" {
		cond = append(cond, "deleted_at IS NOT NULL")
	} else {
		cond = append(cond, "deleted_at IS NULL")
	}

	var sql string
	if len(cond) > 0 {
		for i := range cond {
			cond[i] = strings.Replace(cond[i], "$?", fmt.Sprintf("$%d", i+1), -1)
		}
		sql = strings.Join(cond, " AND ")
	}

	mAdmins := ctrl.backend.ModelByName("Admin")

	count := mAdmins.Where(sql, args...).MustCount()
	admins := []Admin{}
	mAdmins.Find().Where(sql, args...).OrderBy(q.OrderByValue()).Limit(q.Limit()).Offset(q.Offset()).MustQuery(&admins)

	var ids []int
	for _, admin := range admins {
		ids = append(ids, admin.Id)
	}
	var sessionsCountByAdminId map[int]int
	if len(ids) > 0 {
		mAdminSessions := ctrl.backend.ModelByName("AdminSession")
		mAdminSessions.Where("admin_id = ANY($1)", ctrl.backend.toArray(ids)).
			Select("admin_id, COUNT(*)").GroupBy("admin_id").MustQuery(&sessionsCountByAdminId)
	}

	res := struct {
		Admins     []SerializerAdmin
		Pagination pagination.PaginationQuerySortResult
	}{}
	res.Admins = []SerializerAdmin{}
	res.Pagination = q.PaginationQuerySortResult(count)
	for _, admin := range admins {
		a := NewSerializerAdmin(&admin)
		if a == nil {
			continue
		}
		c := sessionsCountByAdminId[a.Id]
		a.SessionsCount = &c
		res.Admins = append(res.Admins, *a)
	}

	return c.JSON(res)
}

func (ctrl fiberAdminsCtrl) Show(c FiberCtx) error {
	var admin Admin
	ctrl.backend.ModelByName("Admin").Find().Where("id = $1", c.Params("id")).MustQuery(&admin)
	u := NewSerializerAdmin(&admin)
	if u != nil {
		c := ctrl.backend.ModelByName("AdminSession").Where("admin_id = $1", admin.Id).MustCount()
		u.SessionsCount = &c
	}
	return c.JSON(u)
}

func (ctrl fiberAdminsCtrl) Create(c FiberCtx) error {
	var admin Admin
	if c.Get("Content-Length") == "0" {
		return c.JSON(NewSerializerAdmin(&admin))
	}
	var id int
	m := ctrl.backend.ModelByName("Admin")
	changes := m.MustAssign(
		&admin,
		m.Permit(ctrl.params()...).Filter(c.Body()),
		m.CreatedAt(),
		m.UpdatedAt(),
	)
	ctrl.backend.MustValidateStruct(admin)
	m.Insert(changes...).Returning("id").MustQueryRow(&id)
	m.Find().Where("id = $1", id).MustQuery(&admin)
	return c.JSON(NewSerializerAdmin(&admin))
}

func (ctrl fiberAdminsCtrl) Update(c FiberCtx) error {
	var admin Admin
	admin.Id, _ = strconv.Atoi(c.Params("id"))
	m := ctrl.backend.ModelByName("Admin")
	changes := m.MustAssign(
		&admin,
		m.Permit(ctrl.params()...).Filter(c.Body()),
		m.UpdatedAt(),
	)
	ctrl.backend.MustValidateStruct(admin)
	m.Update(changes...).Where("id = $1", admin.Id).MustExecute()
	m.Find().Where("id = $1", admin.Id).MustQuery(&admin)
	return c.JSON(NewSerializerAdmin(&admin))
}

func (ctrl fiberAdminsCtrl) Restore(c FiberCtx) error {
	ctrl.backend.ModelByName("Admin").Update("DeletedAt", nil).Where("id = $1", c.Params("id")).MustExecute()
	return ctrl.Show(c)
}

func (ctrl fiberAdminsCtrl) Destroy(c FiberCtx) error {
	ctrl.backend.ModelByName("Admin").Update("DeletedAt", time.Now()).Where("id = $1", c.Params("id")).MustExecute()
	ctrl.backend.ModelByName("AdminSession").Delete().Where("admin_id = $1", c.Params("id")).MustExecute()
	return ctrl.Show(c)
}

func (fiberAdminsCtrl) params() []string {
	return []string{
		"Name", "Password",
	}
}
