/* +build !race */

package metrics

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/rcrowley/go-metrics"
)

// not running this test with race detection because it writes to unsynchronized global variables in an imported
// package
func TestCodaHaleMetricSerialization(t *testing.T) {
	metrics.UseNilMetrics = true
	defer func() { metrics.UseNilMetrics = false }()

	for _, st := range serializationTests {
		m := reflect.ValueOf(st.i).Call(nil)[0].Interface()
		metrics := skipperMetrics{"test": m}
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(metrics)
		var got serializationResult
		json.Unmarshal(buf.Bytes(), &got)

		if !reflect.DeepEqual(got, st.expected) {
			t.Errorf("Got wrong serialization result. Expected '%v' but got '%v'", st.expected, got)
		}

	}
}
