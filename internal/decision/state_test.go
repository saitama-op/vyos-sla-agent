package decision

import (
	"sync"
	"testing"
)

func TestCurrentStateConcurrentAccess(t *testing.T) {
	engine := NewEngine("test")

	var wg sync.WaitGroup
	wg.Add(5)

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			engine.Evaluate(0, 0, 0, 0, 0, 100, 5, 50)
			engine.Evaluate(200, 0, 0, 0, 0, 100, 5, 50)
		}
	}()

	for i := 0; i < 4; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				_ = engine.CurrentState()
			}
		}()
	}

	wg.Wait()
}
