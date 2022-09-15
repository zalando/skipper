package scheduler

var (
	ExportQueueCloseDelay = &queueCloseDelay
)

// GetFifoForTest is only compiled and used in tests
func (r *Registry) GetFifoForTest(s string) (*FifoQueue, bool) {
	id := queueId{
		name:    s,
		grouped: false,
	}
	q, ok := r.fifoQueues[id]
	return q, ok
}
