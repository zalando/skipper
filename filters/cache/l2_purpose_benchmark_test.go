package cache

import (
	"context"
	"testing"
)

var benchmarkL2Context context.Context

func BenchmarkL2Purpose(b *testing.B) {
	ctx := context.Background()
	b.Run("unattributed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkL2Context = ctx
		}
	})
	b.Run("response", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkL2Context = withL2Purpose(ctx, l2PurposeResponse)
		}
	})
}
