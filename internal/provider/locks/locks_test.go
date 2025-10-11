// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package locks

import (
	"sync"
	"testing"
	"time"
)

func TestLockRepo(t *testing.T) {
	repo1 := "repo_123"
	repo2 := "repo_456"

	unlock1 := LockRepo(repo1)
	unlock1()

	unlock1 = LockRepo(repo1)
	unlock2 := LockRepo(repo2)

	unlock1()
	unlock2()
}

func TestLockRepoConcurrency(t *testing.T) {
	repoID := "repo_concurrent_test"
	numGoroutines := 10
	var wg sync.WaitGroup
	var counter int
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			unlock := LockRepo(repoID)
			defer unlock()

			mu.Lock()
			counter++
			mu.Unlock()

			time.Sleep(1 * time.Millisecond)
		}()
	}

	wg.Wait()

	if counter != numGoroutines {
		t.Errorf("Expected counter to be %d, got %d", numGoroutines, counter)
	}
}

func TestLockRepoSequentialAccess(t *testing.T) {
	repoID := "repo_sequential_test"

	start := time.Now()

	unlock1 := LockRepo(repoID)
	time.Sleep(10 * time.Millisecond)
	unlock1()

	unlock2 := LockRepo(repoID)
	unlock2()

	elapsed := time.Since(start)

	if elapsed < 10*time.Millisecond {
		t.Errorf("Expected elapsed time to be at least 10ms due to serialization, got %v", elapsed)
	}
}

func TestLockRepoDifferentReposParallel(t *testing.T) {
	repo1 := "repo_parallel_1"
	repo2 := "repo_parallel_2"

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		unlock := LockRepo(repo1)
		defer unlock()
		time.Sleep(10 * time.Millisecond)
	}()

	go func() {
		defer wg.Done()
		unlock := LockRepo(repo2)
		defer unlock()
		time.Sleep(10 * time.Millisecond)
	}()

	wg.Wait()
	elapsed := time.Since(start)
	if elapsed > 15*time.Millisecond {
		t.Errorf("Expected elapsed time to be close to 10ms (parallel execution), got %v", elapsed)
	}
}

func TestLockRepoReuse(t *testing.T) {
	repoID := "repo_reuse_test"

	unlock1 := LockRepo(repoID)
	unlock1()

	unlock2 := LockRepo(repoID)
	unlock2()

}

func TestLockRepoMultipleCalls(t *testing.T) {
	repoID := "repo_multiple_test"

	unlock1 := LockRepo(repoID)
	unlock1()

	unlock2 := LockRepo(repoID)
	unlock2()

}
