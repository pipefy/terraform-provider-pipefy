// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import "strings"

// splitImportID splits a composite import ID into exactly n non-empty parts.
func splitImportID(id string, n int) ([]string, bool) {
	parts := strings.Split(id, "/")
	if len(parts) != n {
		return nil, false
	}
	for _, p := range parts {
		if p == "" {
			return nil, false
		}
	}
	return parts, true
}
