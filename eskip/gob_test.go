package eskip

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
)

func BenchmarkGobUnmarshal(b *testing.B) {
	var buf bytes.Buffer

	in := MustParse(benchmarkRoutes10k)
	err := gob.NewEncoder(&buf).Encode(in)
	if err != nil {
		b.Fatal(err)
	}

	content := buf.Bytes()

	var out []*Route
	if err := gob.NewDecoder(bytes.NewReader(content)).Decode(&out); err != nil {
		b.Fatal(err)
	}

	if !reflect.DeepEqual(in, out) {
		b.Fatal("input does not match output")
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(float64(len(content)), "bytes/op")

	for i := 0; i < b.N; i++ {
		_ = gob.NewDecoder(bytes.NewReader(content)).Decode(&out)
	}
}
