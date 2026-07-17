// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import "strings"

// splitImportID splits a composite import ID on "/" and reports whether every
// part is non-empty. Callers assert the part count they require.
func splitImportID(id string) ([]string, bool) {
	parts := strings.Split(id, "/")
	for _, p := range parts {
		if p == "" {
			return nil, false
		}
	}
	return parts, true
}
