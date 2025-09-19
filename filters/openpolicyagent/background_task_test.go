package openpolicyagent

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackgroundTaskSystem(t *testing.T) {
	registry, err := NewOpenPolicyAgentRegistry(WithOpenPolicyAgentInstanceConfig(WithConfigTemplate([]byte(""))))
	require.NoError(t, err, "Expected no error creating OpenPolicyAgentRegistry")
	defer registry.Close()

	t.Run("successful task", func(t *testing.T) {
		task, err := registry.ScheduleBackgroundTask(func() error {
			time.Sleep(10 * time.Millisecond) // Simulate some work
			return nil
		})

		assert.NoError(t, err, "Expected no error scheduling task")

		err = task.Wait()
		assert.NoError(t, err, "Expected no error waiting for task")
	})

	t.Run("task with error", func(t *testing.T) {
		expectedError := errors.New("test error")
		task, err := registry.ScheduleBackgroundTask(func() error {
			time.Sleep(10 * time.Millisecond) // Simulate some work
			return expectedError
		})

		assert.NoError(t, err, "Expected no error scheduling task")

		err = task.Wait()
		assert.Error(t, err, "Expected error waiting for task")
		assert.Equal(t, expectedError, err, "Expected error to match")
	})

	t.Run("multiple tasks execute sequentially", func(t *testing.T) {
		var executionOrder []int
		var orderMutex sync.Mutex

		tasks := make([]*BackgroundTask, 3)
		errs := make([]error, 3)
		for i := 0; i < 3; i++ {
			taskNum := i
			tasks[i], errs[i] = registry.ScheduleBackgroundTask(func() error {
				time.Sleep(10 * time.Millisecond) // Simulate some work
				orderMutex.Lock()
				executionOrder = append(executionOrder, taskNum)
				orderMutex.Unlock()
				return nil
			})
		}

		// Wait for all tasks to complete
		for i, task := range tasks {
			err := task.Wait()
			if err != nil {
				t.Fatalf("Task %d failed: %v", i, err)
			}
		}

		// Check that tasks executed sequentially (parallelism = 1)
		if len(executionOrder) != 3 {
			t.Fatalf("Expected 3 tasks to execute, got %d", len(executionOrder))
		}
		// Since parallelism is 1, tasks should execute in order
		for i, taskNum := range executionOrder {
			if taskNum != i {
				t.Fatalf("Tasks did not execute sequentially: expected order [0,1,2], got %v", executionOrder)
			}
		}
	})
}
