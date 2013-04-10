// Copyright 2012 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package database

import (
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/garyburd/gopkgdoc/doc"
)

func isStandardPackage(path string) bool {
	return strings.Index(path, ".") < 0
}

func isTermSep(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func normalizeProjectRoot(projectRoot string) string {
	if projectRoot == "" {
		return "go"
	}
	return projectRoot
}

func suggestPrefix(s string) string {
	var i, n int
	for n < len(s) {
		_, m := utf8.DecodeRuneInString(s[n:])
		n += m
		i += 1
		if i == 2 {
			return s[:n]
		}
	}
	return ""
}

var httpPat = regexp.MustCompile(`https?://\S+`)

func documentTerms(pdoc *doc.Package, rank float64) []string {

	terms := make(map[string]bool)

	// Project root

	projectRoot := normalizeProjectRoot(pdoc.ProjectRoot)
	terms["project:"+projectRoot] = true

	// Imports

	for _, path := range pdoc.Imports {
		if doc.IsValidPath(path) {
			terms["import:"+path] = true
		}
	}

	if rank > 0 {

		if isStandardPackage(pdoc.ImportPath) {
			for _, term := range parseQuery(pdoc.ImportPath) {
				terms[term] = true
			}
		} else {
			terms["all:"] = true
			for _, term := range parseQuery(pdoc.ProjectName) {
				terms[term] = true
			}
			for _, term := range parseQuery(pdoc.Name) {
				terms[term] = true
			}
		}

		// Synopsis

		synopsis := httpPat.ReplaceAllLiteralString(pdoc.Synopsis, "")
		for i, s := range strings.FieldsFunc(synopsis, isTermSep) {
			s = strings.ToLower(s)
			if !stopWord[s] && (i > 3 || s != "package") {
				terms[stem(s)] = true
			}
		}

		if s := suggestPrefix(path.Base(pdoc.ImportPath)); s != "" {
			terms["suggest:"+s] = true
		}
	}

	result := make([]string, 0, len(terms))
	for term := range terms {
		result = append(result, term)
	}
	return result
}

func documentRank(pdoc *doc.Package) float64 {
	if pdoc.Name == "" || pdoc.IsCmd || len(pdoc.Errors) > 0 || strings.HasSuffix(pdoc.ImportPath, ".go") {
		return 0
	}

	for _, p := range pdoc.Imports {
		if strings.HasSuffix(p, ".go") {
			return 0
		}
	}

	if !pdoc.Truncated &&
		len(pdoc.Consts) == 0 &&
		len(pdoc.Vars) == 0 &&
		len(pdoc.Funcs) == 0 &&
		len(pdoc.Types) == 0 &&
		len(pdoc.Examples) == 0 {
		return 0
	}

	var r float64
	switch {
	case isStandardPackage(pdoc.ImportPath):
		r = 1000
	case strings.HasPrefix(pdoc.ImportPath, "code.google.com/p/go."):
		r = 500
	case strings.HasPrefix(pdoc.Synopsis, "Package "+pdoc.Name+" "):
		r = 100
	case len(pdoc.Synopsis) > 0:
		r = 10
	default:
		r = 1
	}

	if strings.Index(pdoc.ImportPath[len(pdoc.ProjectRoot):], "/src/") > 0 {
		r *= 0.85
	}

	if path.Base(pdoc.ImportPath) != pdoc.Name {
		r *= 0.9
	}

	return r
}

func parseQuery(q string) []string {
	var terms []string
	q = strings.ToLower(q)
	for _, s := range strings.FieldsFunc(q, isTermSep) {
		if !stopWord[s] {
			terms = append(terms, stem(s))
		}
	}
	return terms
}
