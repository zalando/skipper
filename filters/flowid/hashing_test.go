package flowid

import "testing"

func TestFlowIdInvalidLength(t *testing.T) {
	_, err := newFlowId(0)
	if err == nil {
		t.Errorf("Request for an invalid flow id length (0) succeeded and it shouldn't")
	}

	_, err = newFlowId(100)
	if err != ErrInvalidLen {
		t.Errorf("Request for an invalid flow id length (100) succeeded and it shouldn't")
	}
}

func TestFlowIdLength(t *testing.T) {
	for expected := MinLength; expected <= MaxLength; expected++ {
		flowId, err := newFlowId(expected)
		if err != nil {
			t.Errorf("Failed to generate flowId with len %d", expected)
		}

		l := len(flowId)
		if l != expected {
			t.Errorf("Got wrong flowId len. Requested %d, got %d (%s)", expected, l, flowId)
		}
	}
}

func BenchmarkFlowIdLen8(b *testing.B) {
	testFlowIdWithLen(b.N, 8)
}

func BenchmarkFlowIdLen10(b *testing.B) {
	testFlowIdWithLen(b.N, 10)
}

func BenchmarkFlowIdLen12(b *testing.B) {
	testFlowIdWithLen(b.N, 12)
}

func BenchmarkFlowIdLen14(b *testing.B) {
	testFlowIdWithLen(b.N, 14)
}

func BenchmarkFlowIdLen16(b *testing.B) {
	testFlowIdWithLen(b.N, 16)
}

func BenchmarkFlowIdLen32(b *testing.B) {
	testFlowIdWithLen(b.N, 32)
}

func BenchmarkFlowIdLen64(b *testing.B) {
	testFlowIdWithLen(b.N, 64)
}

func testFlowIdWithLen(times int, l int) {
	for i := 0; i < times; i++ {
		newFlowId(l)
	}
}
