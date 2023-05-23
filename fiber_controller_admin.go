package backend

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gopsql/pagination/v2"
	"github.com/gopsql/psql"
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
	mAdmins := ctrl.backend.ModelByName("Admin")

	q := pagination.PaginationQuerySort{
		pagination.Pagination{
			MaxPer:     50,
			DefaultPer: 20,
		},
		pagination.Query{},
		pagination.Sort{
			AllowedSorts: map[string]string{
				"name":       mAdmins.ToColumnName("Name"),
				"created_at": mAdmins.ToColumnName("CreatedAt"),
			},
			DefaultSort:  "created_at",
			DefaultOrder: "asc",
		},
	}
	pagination.Bind(&q, c.QueryParser)

	var cond []string
	var args []interface{}
	if pattern := q.GetLikePattern(); pattern != "" {
		var like string
		if mAdmins.Connection() != nil && mAdmins.Connection().DriverName() == "sqlite" {
			like = "LIKE" // SQLite LIKE operator is case-insensitive
		} else {
			like = "ILIKE"
		}
		cond = append(cond, fmt.Sprintf("%s %s $?", mAdmins.ToColumnName("Name"), like))
		args = append(args, pattern)
	}
	if c.Query("status") == "deleted" {
		cond = append(cond, fmt.Sprintf("%s IS NOT NULL", mAdmins.ToColumnName("DeletedAt")))
	} else {
		cond = append(cond, fmt.Sprintf("%s IS NULL", mAdmins.ToColumnName("DeletedAt")))
	}

	var sql string
	if len(cond) > 0 {
		for i := range cond {
			cond[i] = strings.Replace(cond[i], "$?", fmt.Sprintf("$%d", i+1), -1)
		}
		sql = strings.Join(cond, " AND ")
	}

	count := mAdmins.Where(sql, args...).MustCount()
	admins := []Admin{}
	mAdmins.Find().Where(sql, args...).OrderBy(q.OrderByValue()).Limit(q.Limit()).Offset(q.Offset()).MustQuery(&admins)

	var ids []int
	for _, admin := range admins {
		ids = append(ids, admin.Id)
	}
	var sessionsCountByAdminId map[int]int
	if len(ids) > 0 {
		m := ctrl.backend.ModelByName("AdminSession")
		m.Select(m.ToColumnName("AdminId"), "COUNT(*)").Tap(func(s *psql.SelectSQL) *psql.SelectSQL {
			var array []string
			for _, id := range ids {
				array = append(array, strconv.Itoa(id))
			}
			return s.Where(fmt.Sprintf("%s IN (%s)", m.ToColumnName("AdminId"), strings.Join(array, ",")))
		}).GroupBy(m.ToColumnName("AdminId")).MustQuery(&sessionsCountByAdminId)
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
	m := ctrl.backend.ModelByName("Admin")
	m.Find().WHERE("Id", "=", c.Params("id")).MustQuery(&admin)
	u := NewSerializerAdmin(&admin)
	if u != nil {
		c := ctrl.backend.ModelByName("AdminSession").WHERE("AdminId", "=", admin.Id).MustCount()
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
	m.Insert(changes...).Returning(m.ToColumnName("Id")).MustQueryRow(&id)
	m.Find().WHERE("Id", "=", id).MustQuery(&admin)
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
	m.Update(changes...).WHERE("Id", "=", admin.Id).MustExecute()
	m.Find().WHERE("Id", "=", admin.Id).MustQuery(&admin)
	return c.JSON(NewSerializerAdmin(&admin))
}

func (ctrl fiberAdminsCtrl) Restore(c FiberCtx) error {
	m := ctrl.backend.ModelByName("Admin")
	m.Update("DeletedAt", nil).WHERE("Id", "=", c.Params("id")).MustExecute()
	return ctrl.Show(c)
}

func (ctrl fiberAdminsCtrl) Destroy(c FiberCtx) error {
	ma := ctrl.backend.ModelByName("Admin")
	ma.Update("DeletedAt", time.Now().UTC().Truncate(time.Second)).
		WHERE("Id", "=", c.Params("id")).MustExecute()
	mas := ctrl.backend.ModelByName("AdminSession")
	mas.Delete().WHERE("AdminId", "=", c.Params("id")).MustExecute()
	return ctrl.Show(c)
}

func (fiberAdminsCtrl) params() []string {
	return []string{
		"Name", "Password",
	}
}
