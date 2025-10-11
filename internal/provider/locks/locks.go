// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package locks

import "sync"

var repoLocks sync.Map

func LockRepo(repoID string) func() {
	muI, _ := repoLocks.LoadOrStore(repoID, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
