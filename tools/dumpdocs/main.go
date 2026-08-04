// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Command dumpdocs prints every named GraphQL operation the provider sends.
// Documents are constant expressions, so the type checker folds them and this
// reports the exact string that reaches the wire.
package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

var opPattern = regexp.MustCompile(`\b(?:query|mutation)\s+([A-Za-z0-9]+_tf)\b`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	docs, err := loadDocuments()
	if err != nil {
		return err
	}
	if err := reportUnfolded(docs); err != nil {
		return err
	}
	names := make([]string, 0, len(docs))
	for name := range docs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%s\t%s\n", name, docs[name])
	}
	return nil
}

func loadDocuments() (map[string]string, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir: "..",
	}
	pkgs, err := packages.Load(cfg, "github.com/pipefy/terraform-provider-pipefy/internal/...")
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("the provider packages did not type-check")
	}
	docs := map[string]string{}
	var conflicts []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				expr, ok := n.(ast.Expr)
				if !ok {
					return true
				}
				tv, ok := pkg.TypesInfo.Types[expr]
				if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
					return true
				}
				value := constant.StringVal(tv.Value)
				name, ok := documentName(value)
				if !ok {
					return true
				}
				if previous, seen := docs[name]; seen && previous != value {
					conflicts = append(conflicts, fmt.Sprintf("%s:\n  %s\n  %s", name, previous, value))
				}
				docs[name] = value
				return false
			})
		}
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("operation names with more than one document:\n%s", strings.Join(conflicts, "\n"))
	}
	return docs, nil
}

// documentName reports the operation name when value is a whole document. A
// selection fragment or an unrelated string returns false.
func documentName(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "query ") && !strings.HasPrefix(trimmed, "mutation ") {
		return "", false
	}
	match := opPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// reportUnfolded fails when a source file mentions an operation the type
// checker could not fold, which means a document is built at runtime and this
// tool would silently miss it.
func reportUnfolded(docs map[string]string) error {
	var missing []string
	err := filepath.WalkDir("../internal", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range opPattern.FindAllStringSubmatch(string(content), -1) {
			if _, ok := docs[match[1]]; !ok {
				missing = append(missing, fmt.Sprintf("%s in %s", match[1], path))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("operations not folded to a constant: %s", strings.Join(missing, ", "))
	}
	return nil
}
