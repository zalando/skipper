package filters

import "testing"

func TestPathParamCompileFail(t *testing.T) {
	_, err := NewParamTemplate("some {{.invalid template")
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestPathParam(t *testing.T) {
	pt, err := NewParamTemplate("some {{.value}} template")
	if err != nil {
		t.Error(err)
		return
	}

	if b, ok := pt.ExecuteLogged(map[string]string{"value": "working"}); !ok || string(b) != "some working template" {
		t.Error("failed to execute template")
	}
}
