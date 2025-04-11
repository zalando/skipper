package auth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestWithMissingAuth(t *testing.T) {
	spec := NewBasicAuth()
	f, err := spec.CreateFilter([]interface{}{"testdata/htpasswd"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	if err != nil {
		t.Error(err)
	}

	expectedBasicAuthHeaderValue := ForceBasicAuthHeaderValue + `"Basic Realm"`

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if ctx.Response().Header.Get(ForceBasicAuthHeaderName) != expectedBasicAuthHeaderValue && ctx.Response().StatusCode == 401 && ctx.Served() {
		t.Error("Authentication header wrong/missing")
	}
}

func TestWithWrongAuth(t *testing.T) {
	spec := NewBasicAuth()
	f, err := spec.CreateFilter([]interface{}{"testdata/htpasswd", "My Website"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	req.SetBasicAuth("myName", "wrongPassword")
	if err != nil {
		t.Error(err)
	}

	expectedBasicAuthHeaderValue := ForceBasicAuthHeaderValue + `"My Website"`

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if ctx.Response().Header.Get(ForceBasicAuthHeaderName) != expectedBasicAuthHeaderValue && ctx.Response().StatusCode == 401 && ctx.Served() {
		t.Error("Authentication header wrong/missing")
	}
}

func TestWithSuccessfulAuth(t *testing.T) {
	spec := NewBasicAuth()
	f, err := spec.CreateFilter([]interface{}{"testdata/htpasswd"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	req.SetBasicAuth("myName", "myPassword")
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if ctx.Served() && ctx.Response().StatusCode != 401 {
		t.Error("Authentication not successful")
	}
}

func TestWithMissingAuthFile(t *testing.T) {
	spec := NewBasicAuth()
	_, err := spec.CreateFilter([]interface{}{"testdata/missingfile"})
	require.Error(t, err)
	require.Equal(t, "stat failed for \"testdata/missingfile\": stat testdata/missingfile: no such file or directory", err.Error())
}

func TestCreateFilterBasicAuthErrorCases(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []interface{}
		want    filters.Filter
		wantErr bool
	}{
		{
			name:    "test no args passed to filter",
			args:    nil,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "test wrong arg type passed to filter",
			args:    []interface{}{5},
			want:    nil,
			wantErr: true,
		}} {
		t.Run(tt.name, func(t *testing.T) {

			spec := NewBasicAuth()
			got, err := spec.CreateFilter(tt.args)
			if got != tt.want || (tt.wantErr && err == nil) || (!tt.wantErr && err != nil) {
				t.Errorf("Failed to create filter: want %v, got %v, err %v", tt.want, got, err)
			}

		})
	}

}
