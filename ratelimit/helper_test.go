package ratelimit

func repeat(b bool, n int) (result []bool) {
	for range n {
		result = append(result, b)
	}
	return
}
