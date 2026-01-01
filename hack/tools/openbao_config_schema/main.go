package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

func main() {
	var file string
	flag.StringVar(&file, "file", "", "Path to an OpenBao Go source file (e.g. <module>/command/server/config.go)")
	flag.Parse()

	if strings.TrimSpace(file) == "" {
		fmt.Fprintln(os.Stderr, "error: -file is required")
		os.Exit(2)
	}

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: parse %s: %v\n", file, err)
		os.Exit(1)
	}

	keys := make(map[string]struct{})
	ast.Inspect(parsed, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok || field.Tag == nil {
			return true
		}
		raw := strings.Trim(field.Tag.Value, "`")
		tag := reflectStructTag(raw)
		hclTag, ok := tag.Lookup("hcl")
		if !ok {
			return true
		}
		name := strings.TrimSpace(strings.Split(hclTag, ",")[0])
		if name == "" {
			return true
		}
		keys[name] = struct{}{}
		return true
	})

	list := make([]string, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	for _, k := range list {
		fmt.Println(k)
	}
}

// reflectStructTag is a tiny subset of reflect.StructTag to avoid importing reflect
// (and to keep behavior predictable for our simple parsing needs).
type reflectStructTag string

func (tag reflectStructTag) Lookup(key string) (string, bool) {
	// Copied in spirit from reflect.StructTag.Lookup, simplified for our use-case.
	s := string(tag)
	for s != "" {
		s = strings.TrimLeft(s, " ")
		if s == "" {
			break
		}
		i := strings.IndexByte(s, ':')
		if i <= 0 {
			break
		}
		name := s[:i]
		s = s[i+1:]
		if !strings.HasPrefix(s, "\"") {
			break
		}
		s = s[1:]
		j := 0
		for j < len(s) {
			if s[j] == '"' {
				// tag values can contain escaped quotes
				if j > 0 && s[j-1] == '\\' {
					j++
					continue
				}
				break
			}
			j++
		}
		if j >= len(s) {
			break
		}
		value := s[:j]
		s = s[j+1:]
		if name == key {
			return value, true
		}
	}
	return "", false
}
