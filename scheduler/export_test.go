package scheduler

var (
	ExportQueueCloseDelay = &queueCloseDelay
)

// GetFifoForeTest is only compiled and used in tests
func (r *Registry) GetFifoForeTest(s string) (*FifoQueue, bool) {
	id := queueId{
		name:    s,
		grouped: false,
	}
	q, ok := r.fifoQueues[id]
	return q, ok
}
