// Copyright 2011 Gary Burd
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

package app

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"strings"
	"time"
	"unicode"
)

var errPackageNotFound = os.NewError("package not found")

func includeFileInDoc(p string) bool {
	_, f := path.Split(p)
	return strings.HasSuffix(f, ".go") &&
		!strings.HasSuffix(f, "_test.go") &&
		len(f) > 0 &&
		f[0] != '_' &&
		f[0] != '.' &&
		f != "deprecated.go"
}

func synopsis(s string) string {
	// Split off first paragraph.
	if parts := strings.SplitN(s, "\n\n", 2); len(parts) > 1 {
		s = parts[0]
	}

	// Find first sentence.
	prev := 'A'
	for i, ch := range s {
		if (ch == '.' || ch == '!' || ch == '?') &&
			i+1 < len(s) &&
			(s[i+1] == ' ' || s[i+1] == '\n') &&
			!unicode.IsUpper(prev) {
			s = s[:i+1]
			break
		}
		prev = ch
	}

	// Ensure that synopsis fits in datastore text property.
	if len(s) > 400 {
		s = s[:400]
	}

	return s
}

func printNode(fset *token.FileSet, decl interface{}) string {
	var buf bytes.Buffer
	_, err := (&printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}).Fprint(&buf, fset, decl)
	if err != nil {
		buf.Reset()
		buf.WriteString(err.String())
	}
	return buf.String()
}

func printPos(fset *token.FileSet, lineFmt string, pos token.Pos) string {
	position := fset.Position(pos)
	return position.Filename + fmt.Sprintf(lineFmt, position.Line)
}

type valueDoc struct {
	Decl string
	URL  string
	Doc  string
}

func valueDocs(fset *token.FileSet, lineFmt string, values []*doc.ValueDoc) []*valueDoc {
	var docs []*valueDoc
	for _, d := range values {
		docs = append(docs, &valueDoc{
			Decl: printNode(fset, d.Decl),
			URL:  printPos(fset, lineFmt, d.Decl.Pos()),
			Doc:  d.Doc,
		})
	}
	return docs
}

type funcDoc struct {
	Decl string
	URL  string
	Doc  string
	Name string
	Recv string
}

func funcDocs(fset *token.FileSet, lineFmt string, funcs []*doc.FuncDoc) []*funcDoc {
	var docs []*funcDoc
	for _, d := range funcs {
		recv := ""
		if d.Recv != nil {
			recv = printNode(fset, d.Recv)
		}
		docs = append(docs, &funcDoc{
			Decl: printNode(fset, d.Decl),
			URL:  printPos(fset, lineFmt, d.Decl.Pos()),
			Doc:  d.Doc,
			Name: d.Name,
			Recv: recv,
		})
	}
	return docs
}

type typeDoc struct {
	Doc       string
	Name      string
	Decl      string
	URL       string
	Factories []*funcDoc
	Methods   []*funcDoc
}

func typeDocs(fset *token.FileSet, lineFmt string, types []*doc.TypeDoc) []*typeDoc {
	var docs []*typeDoc
	for _, d := range types {
		docs = append(docs, &typeDoc{
			Doc:       d.Doc,
			Name:      printNode(fset, d.Type.Name),
			Decl:      printNode(fset, d.Decl),
			URL:       printPos(fset, lineFmt, d.Decl.Pos()),
			Factories: funcDocs(fset, lineFmt, d.Factories),
			Methods:   funcDocs(fset, lineFmt, d.Methods),
		})
	}
	return docs
}

type fileDoc struct {
	Name string
	URL  string
}

func fileDocs(urls []string) []*fileDoc {
	var docs []*fileDoc
	for _, url := range urls {
		_, name := path.Split(url)
		docs = append(docs, &fileDoc{
			Name: name,
			URL:  url,
		})
	}
	return docs
}

type packageDoc struct {
	Consts      []*valueDoc
	Doc         string
	Synopsis    string
	Files       []*fileDoc
	Funcs       []*funcDoc
	ImportPath  string
	PackageName string
	Types       []*typeDoc
	Updated     int64
	Vars        []*valueDoc
	ProjectURL  string
	ProjectName string
}

type file struct {
	url     string
	content interface{}
}

func createPackageDoc(importPath string, lineFmt string, files []file) (*packageDoc, os.Error) {
	if len(files) == 0 {
		return nil, errPackageNotFound
	}

	fset := token.NewFileSet()
	pkgs := make(map[string]*ast.Package)
	for _, f := range files {
		if src, err := parser.ParseFile(fset, f.url, f.content, parser.ParseComments); err == nil {
			name := src.Name.Name
			pkg, found := pkgs[name]
			if !found {
				pkg = &ast.Package{name, nil, nil, make(map[string]*ast.File)}
				pkgs[name] = pkg
			}
			pkg.Files[f.url] = src
		}
	}
	var pkg *ast.Package
	score := 0
	for _, p := range pkgs {
		switch {
		case score < 3 && strings.HasSuffix(importPath, p.Name):
			pkg = p
			score = 3
		case score < 2 && p.Name != "main":
			pkg = p
			score = 2
		case score < 1:
			pkg = p
			score = 1
		}
	}

	if pkg == nil {
		return nil, errPackageNotFound
	}

	ast.PackageExports(pkg)
	pdoc := doc.NewPackageDoc(pkg, importPath)

	return &packageDoc{
		Consts:      valueDocs(fset, lineFmt, pdoc.Consts),
		Doc:         pdoc.Doc,
		Synopsis:    synopsis(pdoc.Doc),
		Files:       fileDocs(pdoc.Filenames),
		Funcs:       funcDocs(fset, lineFmt, pdoc.Funcs),
		ImportPath:  pdoc.ImportPath,
		PackageName: pdoc.PackageName,
		Types:       typeDocs(fset, lineFmt, pdoc.Types),
		Updated:     time.Seconds(),
		Vars:        valueDocs(fset, lineFmt, pdoc.Vars),
	}, nil
}
