package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type packageInfo struct {
	ImportPath   string
	Dir          string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
}

type symbolKey struct {
	ImportPath string
	Name       string
}

type declaration struct {
	Key  symbolKey
	Kind string
	File string
	Line int
}

type finding struct {
	Decl     declaration
	TestRefs int
}

func main() {
	flag.Parse()

	modulePath, err := goListModule()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testonly_exports: %v\n", err)
		os.Exit(1)
	}

	packages, err := goListPackages("./cmd/...", "./internal/...")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testonly_exports: %v\n", err)
		os.Exit(1)
	}

	findings, err := analyzePackages(modulePath, packages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testonly_exports: %v\n", err)
		os.Exit(1)
	}
	if len(findings) == 0 {
		return
	}

	fmt.Fprintln(os.Stderr, "testonly_exports: exported production declarations referenced only from tests:")
	for _, finding := range findings {
		decl := finding.Decl
		fmt.Fprintf(
			os.Stderr,
			"  %s:%d: %s %s.%s, test refs=%d\n",
			filepath.ToSlash(decl.File),
			decl.Line,
			decl.Kind,
			decl.Key.ImportPath,
			decl.Key.Name,
			finding.TestRefs,
		)
	}
	os.Exit(1)
}

func goListModule() (string, error) {
	cmd := exec.Command("go", "list", "-m")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list -m: %w", err)
	}
	modulePath := strings.TrimSpace(string(out))
	if modulePath == "" {
		return "", fmt.Errorf("go list -m returned an empty module path")
	}
	return modulePath, nil
}

func goListPackages(patterns ...string) ([]packageInfo, error) {
	args := append([]string{"list", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	var packages []packageInfo
	for {
		var pkg packageInfo
		if err := dec.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go list package: %w", err)
		}
		packages = append(packages, pkg)
	}
	return packages, nil
}

func analyzePackages(modulePath string, packages []packageInfo) ([]finding, error) {
	declarations := make(map[symbolKey]declaration)
	prodRefs := make(map[symbolKey]int)
	testRefs := make(map[symbolKey]int)

	for _, pkg := range packages {
		if skipPackage(modulePath, pkg.ImportPath) {
			continue
		}

		for _, name := range pkg.GoFiles {
			filePath := filepath.Join(pkg.Dir, name)
			file, fset, generated, err := parseGoFile(filePath)
			if err != nil {
				return nil, err
			}
			declPositions := collectDeclarationPositions(file)
			if !generated {
				for _, decl := range collectDeclarations(pkg.ImportPath, filePath, file, fset) {
					declarations[decl.Key] = decl
				}
			}
			countReferences(modulePath, pkg.ImportPath, file, declPositions, prodRefs)
		}

		for _, name := range append(pkg.TestGoFiles, pkg.XTestGoFiles...) {
			filePath := filepath.Join(pkg.Dir, name)
			file, _, _, err := parseGoFile(filePath)
			if err != nil {
				return nil, err
			}
			countReferences(modulePath, pkg.ImportPath, file, nil, testRefs)
		}
	}

	keys := make([]symbolKey, 0, len(declarations))
	for key := range declarations {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ImportPath != keys[j].ImportPath {
			return keys[i].ImportPath < keys[j].ImportPath
		}
		return keys[i].Name < keys[j].Name
	})

	var findings []finding
	for _, key := range keys {
		if prodRefs[key] == 0 && testRefs[key] > 0 {
			findings = append(findings, finding{
				Decl:     declarations[key],
				TestRefs: testRefs[key],
			})
		}
	}
	return findings, nil
}

func skipPackage(modulePath, importPath string) bool {
	rel := strings.TrimPrefix(importPath, modulePath+"/")
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == "testutil" {
			return true
		}
	}
	return false
}

func parseGoFile(filePath string) (*ast.File, *token.FileSet, bool, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, false, fmt.Errorf("read %s: %w", filePath, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, nil, false, fmt.Errorf("parse %s: %w", filePath, err)
	}
	return file, fset, isGenerated(src), nil
}

func isGenerated(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//") {
			return false
		}
		if strings.Contains(trimmed, "Code generated") && strings.Contains(trimmed, "DO NOT EDIT") {
			return true
		}
	}
	return false
}

func collectDeclarationPositions(file *ast.File) map[token.Pos]struct{} {
	positions := make(map[token.Pos]struct{})
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Name != nil {
				positions[decl.Name.Pos()] = struct{}{}
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					positions[spec.Name.Pos()] = struct{}{}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						positions[name.Pos()] = struct{}{}
					}
				}
			}
		}
	}
	return positions
}

func collectDeclarations(importPath, filePath string, file *ast.File, fset *token.FileSet) []declaration {
	var declarations []declaration
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil && decl.Name != nil && ast.IsExported(decl.Name.Name) {
				declarations = append(
					declarations,
					newDeclaration(importPath, decl.Name.Name, "func", filePath, fset, decl.Name.Pos()),
				)
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if ast.IsExported(spec.Name.Name) {
						declarations = append(
							declarations,
							newDeclaration(importPath, spec.Name.Name, "type", filePath, fset, spec.Name.Pos()),
						)
					}
				case *ast.ValueSpec:
					kind := strings.ToLower(decl.Tok.String())
					for _, name := range spec.Names {
						if ast.IsExported(name.Name) {
							declarations = append(declarations, newDeclaration(importPath, name.Name, kind, filePath, fset, name.Pos()))
						}
					}
				}
			}
		}
	}
	return declarations
}

func newDeclaration(importPath, name, kind, filePath string, fset *token.FileSet, pos token.Pos) declaration {
	return declaration{
		Key: symbolKey{
			ImportPath: importPath,
			Name:       name,
		},
		Kind: kind,
		File: filePath,
		Line: fset.Position(pos).Line,
	}
}

func countReferences(
	modulePath string,
	importPath string,
	file *ast.File,
	declPositions map[token.Pos]struct{},
	refs map[symbolKey]int,
) {
	imports := importAliases(file)
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case nil:
			return true
		case *ast.ImportSpec:
			return false
		case *ast.SelectorExpr:
			if ident, ok := node.X.(*ast.Ident); ok {
				if importedPath, ok := imports[ident.Name]; ok && isModuleImport(modulePath, importedPath) {
					refs[symbolKey{ImportPath: importedPath, Name: node.Sel.Name}]++
				}
				return false
			}
			return true
		case *ast.Ident:
			if _, ok := declPositions[node.Pos()]; ok {
				return true
			}
			if ast.IsExported(node.Name) {
				refs[symbolKey{ImportPath: importPath, Name: node.Name}]++
			}
		}
		return true
	})
}

func importAliases(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "_" || spec.Name.Name == "." {
				continue
			}
			imports[spec.Name.Name] = importPath
			continue
		}
		imports[path.Base(importPath)] = importPath
	}
	return imports
}

func isModuleImport(modulePath, importPath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}
