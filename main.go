package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// https://golang.org/ref/spec#Predeclared_identifiers
var universe = func() map[string]struct{} {
	m := make(map[string]struct{})
	ids := []string{
		// Types:
		"bool", "byte", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",

		// Constants:
		"true", "false", "iota",

		// Zero value:
		"nil",

		// Functions:
		"append", "cap", "close", "complex", "copy", "delete", "imag", "len",
		"make", "new", "panic", "print", "println", "real", "recover",
	}

	for _, s := range ids {
		m[s] = struct{}{}
	}

	return m
}()

func main() {

	var seen sync.Map
	var wg sync.WaitGroup

	if len(os.Args) == 1 {
		fmt.Println("Supply a directory argument")
		os.Exit(1)
	}
	path := os.Args[1]

	wg.Add(1)
	go pkgWalk(&wg, &seen, path)
	wg.Wait()
}

func pkgWalk(wg *sync.WaitGroup, visited *sync.Map, path string) {
	defer wg.Done()

	if _, v := visited.Load(path); v {
		return
	}
	visited.Store(path, true)

	pkg, err := build.ImportDir(path, 0)
	if err == nil {
		for _, f := range pkg.GoFiles {
			check(filepath.Join(pkg.Dir, f))
		}
	}

	files, err := ioutil.ReadDir(path)
	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		wg.Add(1)
		go pkgWalk(wg, visited, filepath.Join(path, f.Name()))
	}
}

func check(file string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	ast.Inspect(f, func(n ast.Node) bool {
		var violations []ast.Node

		switch x := n.(type) {
		case *ast.AssignStmt:
			violations = checkAssign(x)
		case *ast.GenDecl:
			violations = checkDecl(x)
		}

		for _, v := range violations {
			report(v, fset)
		}

		return true
	})
}

func report(node ast.Node, fset *token.FileSet) {
	pos := fset.Position(node.Pos())

	var buf bytes.Buffer
	_ = format.Node(&buf, fset, node)
	fmt.Printf("%s:\n%s\n\n", pos.String(), buf.String())
}

func checkAssign(s *ast.AssignStmt) (found []ast.Node) {
	found = []ast.Node{}

	if s.Tok != token.DEFINE {
		return
	}

	for _, expr := range s.Lhs {
		id, ok := expr.(*ast.Ident)
		if !ok {
			continue
		}

		if shadowed(id) {
			found = append(found, s)
		}
	}

	return
}

func checkDecl(d *ast.GenDecl) (found []ast.Node) {
	found = []ast.Node{}
	if d.Tok != token.VAR {
		return
	}

	for _, spec := range d.Specs {
		v, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, id := range v.Names {
			if shadowed(id) {
				found = append(found, d)
			}
		}
	}

	return
}

func shadowed(id *ast.Ident) bool {
	_, ok := universe[id.Name]
	return ok
}
