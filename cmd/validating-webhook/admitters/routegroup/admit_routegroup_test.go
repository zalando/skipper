package routegroup

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	zv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type testRunner struct {
}

type testcaseDefinition struct {
	Operation v1beta1.Operation `json:"operation"`
	Object    interface{}       `json:"object"`
	Error     string            `json:'error'`
}

func (r *testRunner) runTestcase(t *testing.T, filename string) {
	data, err := ioutil.ReadFile(filename)
	require.NoError(t, err)

	var testcase testcaseDefinition

	err = yaml.Unmarshal(data, &testcase)
	require.NoError(t, err)

	objectYaml, err := yaml.Marshal(testcase.Object)
	require.NoError(t, err)
	object := r.parseObject(t, string(objectYaml))

	ar := r.objectToRequest(t, object)

	resp := admit(ar)

	if testcase.Error != "" {
		require.Equal(t, false, resp.Allowed)
		require.Equal(t, testcase.Error, resp.Result.Message)
	}

}

func (r *testRunner) runTestCases(t *testing.T) {
	files, err := ioutil.ReadDir("testcases")
	require.NoError(t, err)

	r.RunTestCasesForFiles(t, files)
}

func (r *testRunner) RunTestCasesForFiles(t *testing.T, files []os.FileInfo) {
	for _, file := range files {
		t.Run(file.Name(), func(t *testing.T) {
			r.runTestcase(t, path.Join("testcases", file.Name()))
		})
	}
}

func (r *testRunner) parseObject(t *testing.T, from string) runtime.Object {
	object := &zv1.RouteGroup{}
	_, _, err := deserializer.Decode([]byte(from), nil, object)
	require.NoError(t, err)
	return object
}

func (r *testRunner) objectToRequest(t *testing.T, object runtime.Object) *v1beta1.AdmissionRequest {
	serialized, err := json.Marshal(object)
	require.NoError(t, err)

	return &v1beta1.AdmissionRequest{
		UID:  "example",
		Kind: metav1.GroupVersionKind{Group: "zalando.org", Version: "v1", Kind: "RouteGroup"},
		Object: runtime.RawExtension{
			Raw: serialized,
		},
	}
}

func TestRouteGroupAdmission(t *testing.T) {
	runner := &testRunner{}

	runner.runTestCases(t)
}
