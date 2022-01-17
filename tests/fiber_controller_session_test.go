package backend

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopsql/backend"
)

func TestSignInAndSignOut(_t *testing.T) {
	t := &test{_t}
	testWithSqlite(func() {
		testSignInAndSignOut(t)
	})
}

func testSignInAndSignOut(t *test) {
	name, password, _ := backend.Default.CreateAdmin("admin", "123123")
	t.String("admin name", name, "admin")
	t.String("admin password", password, "123123")

	var resBody json.RawMessage

	t.Request(httptest.NewRequest("GET", "/admins", nil), 401, &resBody)
	t.String("response", string(resBody), `{"Message":"Please Log In"}`)

	t.Request(httptest.NewRequest("GET", "/me", nil), 200, &resBody)
	t.String("response", string(resBody), `null`)

	t.Request(httptest.NewRequest("POST", "/sign-out", nil), 401, &resBody)
	t.String("response", string(resBody), `{"Message":"Please Log In"}`)

	t.Request(httptest.NewRequest("POST", "/sign-in", strings.NewReader(`{}`)), 400, &resBody)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Name","Name":"Name","Kind":"string","Type":"gt","Param":"0"},`+
			`{"FullName":"Password","Name":"Password","Kind":"string","Type":"gte","Param":"6"}]}`)

	t.Request(httptest.NewRequest("POST", "/sign-in", strings.NewReader(`{ "Name": "admin" }`)), 400, &resBody)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Password","Name":"Password","Kind":"string","Type":"gte","Param":"6"}]}`)

	t.Request(httptest.NewRequest("POST", "/sign-in", strings.NewReader(`{ "Name": "admin", "Password": "123456" }`)), 400, &resBody)
	t.String("response", string(resBody),
		`{"Errors":[{"FullName":"Password","Name":"Password","Kind":"string","Type":"wrong","Param":""}]}`)

	var token tokenResponse
	t.Request(httptest.NewRequest("POST", "/sign-in", strings.NewReader(`{ "Name": "admin", "Password": "123123" }`)), 200, &token)
	t.Bool("token size greater than 0", len(token.Token) > 0, true)

	req := httptest.NewRequest("GET", "/admins", nil)
	t.Request(req, 200, nil, token)

	t.Request(httptest.NewRequest("GET", "/me", nil), 200, &resBody, token)
	t.String("response", string(resBody), `{"Id":1,"Name":"admin"}`)

	t.Request(httptest.NewRequest("POST", "/sign-out", nil), 204, nil, token)

	t.Request(httptest.NewRequest("GET", "/admins", nil), 401, &resBody, token)
	t.String("response", string(resBody), `{"Message":"Please Log In"}`)

	t.Request(httptest.NewRequest("GET", "/me", nil), 200, &resBody, token)
	t.String("response", string(resBody), `null`)
}
