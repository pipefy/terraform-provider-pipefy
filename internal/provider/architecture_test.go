// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// graphQLDocument matches the start of a named operation. It requires the
// keyword, so a test double comparing against a bare operation name does not
// trip it.
var graphQLDocument = regexp.MustCompile(`\b(?:query|mutation)\s+[A-Za-z0-9]+_tf\b`)

// TestNoGraphQLOutsideSDK fails when a GraphQL document appears anywhere but
// internal/pipefy. Every operation living in one package is what lets the API
// layer be exercised without a Terraform harness, and it is easy to undo by
// accident: writing a query straight into a resource is the shape the provider
// had before, and nothing else in the build would object.
//
// depguard covers the other direction, stopping the SDK from importing
// Terraform. This covers this one.
func TestNoGraphQLOutsideSDK(t *testing.T) {
	// The test's working directory is internal/provider, so this walks internal/.
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasPrefix(filepath.ToSlash(path), "../pipefy/") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if match := graphQLDocument.Find(content); match != nil {
			t.Errorf("%s declares %q; GraphQL documents belong in internal/pipefy", path, match)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
}
