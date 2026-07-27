package version

import (
	"fmt"
	"sync"
	"testing"
)

func TestGetSet(t *testing.T) {
	prev := Get()
	t.Cleanup(func() { Set(prev) })

	if got := Get(); got != "dev" {
		t.Fatalf("Get() before Set() = %q, want %q", got, "dev")
	}

	Set("1.2.3")
	if got := Get(); got != "1.2.3" {
		t.Fatalf("Get() after Set() = %q, want %q", got, "1.2.3")
	}

	Set("")
	if got := Get(); got != "1.2.3" {
		t.Fatalf("Get() after empty Set() = %q, want prior value %q", got, "1.2.3")
	}

	Set("  2.0.0  ")
	if got := Get(); got != "2.0.0" {
		t.Fatalf("Get() after whitespace Set() = %q, want %q", got, "2.0.0")
	}
}

func TestConcurrentGetSet(t *testing.T) {
	prev := Get()
	Set("dev")
	t.Cleanup(func() { Set(prev) })

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			Set(fmt.Sprintf("version-%d", i))
		}(i)
		go func() {
			defer wg.Done()
			if got := Get(); got == "" {
				t.Error("Get() returned empty string during concurrent access")
			}
		}()
	}
	wg.Wait()

	if got := Get(); got == "" {
		t.Fatal("Get() returned empty string after concurrent access")
	}
}
