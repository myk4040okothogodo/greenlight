package main

import (
	"fmt"
	"sync"
)

func main() {
	// Declare a new WaitGroup.
	var wg sync.WaitGroup

	// Execute a loop 5 times
	for i := 1; i <= 5; i++ {
		// Increment the WaitGroup counter by 1, BEFORE we launch the background routine.
		wg.Add(1)

		// Launch the background goroutine
		go func() {
			// Defer a call to wg.Done() to indicate that the background goroutine has completed when this function returns. Behinf the scenes this
			// decrements the WaitGroup counter by 1 and is the same as writing wg.Add(-1)
			defer wg.Done()

			fmt.Println("hello from a goroutine")
		}()
	}

	// Wait() blocks untill the WaitGroup counter is zero  ---essentially blocking untill all go routines have completed
	wg.Wait()

	fmt.Println("all goroutines finished.")
}
