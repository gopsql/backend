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
	admins := mAdmins.NewSlice()
	mAdmins.Find().Where(sql, args...).OrderBy(q.OrderByValue()).Limit(q.Limit()).Offset(q.Offset()).MustQuery(admins.Interface())

	ret := struct {
		Admins        []interface{}
		SessionsCount map[int]int
		Pagination    pagination.PaginationQuerySortResult
	}{Pagination: q.PaginationQuerySortResult(count)}

	var ids []string
	for i := 0; i < admins.Elem().Len(); i++ {
		elem := admins.Elem().Index(i).Addr().Interface()
		if admin, ok := elem.(Serializable); ok {
			ret.Admins = append(ret.Admins, admin.Serialize("list"))
		} else {
			ret.Admins = append(ret.Admins, elem)
		}
		if admin, ok := elem.(IsAdmin); ok {
			ids = append(ids, strconv.Itoa(admin.GetId()))
		}
	}

	if len(ids) > 0 {
		m := ctrl.backend.ModelByName("AdminSession")
		m.Select(m.ToColumnName("AdminId"), "COUNT(*)").Tap(func(s *psql.SelectSQL) *psql.SelectSQL {
			return s.Where(fmt.Sprintf("%s IN (%s)", m.ToColumnName("AdminId"), strings.Join(ids, ",")))
		}).GroupBy(m.ToColumnName("AdminId")).MustQuery(&ret.SessionsCount)
	}

	return c.JSON(ret)
}

func (ctrl fiberAdminsCtrl) Show(c FiberCtx) error {
	m := ctrl.backend.ModelByName("Admin")
	admin := m.New().Interface()
	m.Find().WHERE("Id", "=", c.Params("id")).MustQuery(admin)
	if u, ok := admin.(Serializable); ok {
		return c.JSON(u.Serialize("show"))
	}
	return c.JSON(admin)
}

func (ctrl fiberAdminsCtrl) Create(c FiberCtx) error {
	m := ctrl.backend.ModelByName("Admin")
	admin := m.New().Interface()
	if c.Get("Content-Length") == "0" {
		if u, ok := admin.(Serializable); ok {
			return c.JSON(u.Serialize("show"))
		}
		return c.JSON(admin)
	}
	var id int
	changes := m.MustAssign(
		admin,
		m.Permit(ctrl.params("create")...).Filter(c.Body()),
		m.CreatedAt(),
		m.UpdatedAt(),
	)
	ctrl.backend.MustValidateStruct(admin)
	m.Insert(changes...).Returning(m.ToColumnName("Id")).MustQueryRow(&id)
	m.Find().WHERE("Id", "=", id).MustQuery(admin)
	return c.JSON(admin)
}

func (ctrl fiberAdminsCtrl) Update(c FiberCtx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	m := ctrl.backend.ModelByName("Admin")
	admin := m.New().Interface()
	if admin, ok := admin.(IsAdmin); ok {
		admin.SetId(id)
	}
	changes := m.MustAssign(
		admin,
		m.Permit(ctrl.params("update")...).Filter(c.Body()),
		m.UpdatedAt(),
	)
	ctrl.backend.MustValidateStruct(admin)
	m.Update(changes...).WHERE("Id", "=", id).MustExecute()
	m.Find().WHERE("Id", "=", id).MustQuery(admin)
	return c.JSON(admin)
}

func (ctrl fiberAdminsCtrl) Restore(c FiberCtx) error {
	ctrl.backend.ModelByName("Admin").Update("DeletedAt", nil).WHERE("Id", "=", c.Params("id")).MustExecute()
	return ctrl.Show(c)
}

func (ctrl fiberAdminsCtrl) Destroy(c FiberCtx) error {
	ctrl.backend.ModelByName("Admin").
		Update("DeletedAt", time.Now().UTC().Truncate(time.Second)).
		WHERE("Id", "=", c.Params("id")).MustExecute()
	ctrl.backend.ModelByName("AdminSession").
		Delete().WHERE("AdminId", "=", c.Params("id")).MustExecute()
	return ctrl.Show(c)
}

func (ctrl fiberAdminsCtrl) params(action string) []string {
	admin := ctrl.backend.ModelByName("Admin").New().Interface()
	if admin, ok := admin.(HasParams); ok {
		return admin.Params(action)
	}
	return []string{"Name", "Password"}
}
