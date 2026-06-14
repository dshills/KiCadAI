package libraryresolver

import (
	"context"
	"runtime"
	"sync"
)

func parallelMap[T any](ctx context.Context, count int, fn func(index int) T) []T {
	results := make([]T, count)
	if count == 0 {
		return results
	}
	if ctx == nil {
		ctx = context.Background()
	}
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount > count {
		workerCount = count
	}
	jobs := make(chan int)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerCount)
	for range workerCount {
		go func() {
			defer waitGroup.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				results[index] = fn(index)
			}
		}()
	}
	for index := range count {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			waitGroup.Wait()
			return results
		}
	}
	close(jobs)
	waitGroup.Wait()
	return results
}
