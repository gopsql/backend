package backend

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gopsql/backend"
)

func TestAdmins(_t *testing.T) {
	t := &test{_t}
	testWithSqlite(func() {
		testAdmins(t)
	})
}

func testAdmins(t *test) {
	name, password, _ := backend.Default.CreateAdmin("admin", "")

	var token tokenResponse
	t.Request(httptest.NewRequest("POST", "/sign-in", asJson(struct {
		Name     string
		Password string
	}{name, password})), 200, &token)
	t.Bool("token size greater than 0", len(token.Token) > 0, true)

	type Admin struct {
		Id        int
		Name      string
		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt *time.Time
	}

	var list struct {
		Admins []Admin
	}
	t.Request(httptest.NewRequest("GET", "/admins", nil), 200, &list, token)
	t.Int("list size", len(list.Admins), 1)

	var resBody json.RawMessage
	t.Request(httptest.NewRequest("POST", "/admins", strings.NewReader(`{ "Name": "" }`)), 400, &resBody, token)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Admin.Name","Name":"Name","Kind":"string","Type":"gt","Param":"0"}]}`)

	t.Request(httptest.NewRequest("POST", "/admins", strings.NewReader(`{ "Name": "admin", "Password": "123456" }`)), 400, &resBody, token)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Admin.Name","Name":"Name","Kind":"string","Type":"uniqueness","Param":""}]}`)

	t.Request(httptest.NewRequest("POST", "/admins", strings.NewReader(`{ "Name": "foobar", "Password": "123" }`)), 400, &resBody, token)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Admin.Password.Password","Name":"Password","Kind":"string","Type":"gte","Param":"6"}]}`)

	var newAdmin Admin
	t.Request(httptest.NewRequest("POST", "/admins", strings.NewReader(`{ "Name": "foobar", "Password": "123123" }`)), 200, &newAdmin, token)
	t.Int("admin id", newAdmin.Id, 2)
	t.String("admin name", newAdmin.Name, "foobar")

	adminPath := fmt.Sprintf("/admins/%d", newAdmin.Id)

	var showAdmin Admin
	t.Request(httptest.NewRequest("GET", adminPath, nil), 200, &showAdmin, token)
	t.Bool("admin equal", newAdmin == showAdmin, true)

	t.Request(httptest.NewRequest("GET", "/admins", nil), 200, &list, token)
	t.Int("list size", len(list.Admins), 2)
	t.String("admin name", list.Admins[0].Name, "admin")
	t.String("admin name", list.Admins[1].Name, "foobar")

	t.Request(httptest.NewRequest("GET", "/admins?sort=created_at&order=desc", nil), 200, &list, token)
	t.String("admin name", list.Admins[0].Name, "foobar")

	t.Request(httptest.NewRequest("GET", "/admins?query=FOO", nil), 200, &list, token)
	t.Int("list size", len(list.Admins), 1)
	t.String("admin name", list.Admins[0].Name, "foobar")

	t.Request(httptest.NewRequest("PUT", adminPath, strings.NewReader(`{ "Name": "ADMIN" }`)), 400, &resBody, token)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Admin.Name","Name":"Name","Kind":"string","Type":"uniqueness","Param":""}]}`)

	var updateAdmin Admin
	t.Request(httptest.NewRequest("PUT", adminPath, strings.NewReader(`{ "Name": "root" }`)), 200, &updateAdmin, token)
	t.String("admin name", updateAdmin.Name, "root")
	t.Bool("admin deleted at is null", updateAdmin.DeletedAt == nil, true)

	t.Request(httptest.NewRequest("POST", "/sign-in", strings.NewReader(`{ "Name": "root", "Password": "123123" }`)), 200, nil)

	var deletedAdmin Admin
	t.Request(httptest.NewRequest("DELETE", adminPath, nil), 200, &deletedAdmin, token)
	t.Bool("admin deleted at is not null", deletedAdmin.DeletedAt != nil, true)

	t.Request(httptest.NewRequest("POST", "/sign-in", strings.NewReader(`{ "Name": "root", "Password": "123123" }`)), 400, &resBody)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Name","Name":"Name","Kind":"string","Type":"deleted","Param":""}]}`)

	var restoredAdmin Admin
	t.Request(httptest.NewRequest("POST", adminPath, nil), 200, &restoredAdmin, token)
	t.Bool("admin deleted at is null", restoredAdmin.DeletedAt == nil, true)

	t.Request(httptest.NewRequest("POST", "/sign-in", strings.NewReader(`{ "Name": "root", "Password": "123123" }`)), 200, nil)
}
