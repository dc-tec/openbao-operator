package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type rootSpec struct {
	Prefix string
	Dir    string
	Type   string
}

func main() {
	var openbaoImageTag string
	var openbaoGitSHA string
	var maxList int
	var verbose bool

	flag.StringVar(&openbaoImageTag, "openbao-image-tag", "2.4.4", "OpenBao Docker image tag used to resolve an upstream git SHA (e.g. 2.4.4)")
	flag.StringVar(&openbaoGitSHA, "openbao-git-sha", "", "OpenBao git SHA (40 hex) to use instead of resolving from Docker image")
	flag.IntVar(&maxList, "max-list", 60, "Max number of keys to print per section (0 = unlimited)")
	flag.BoolVar(&verbose, "v", false, "Verbose output (prints extra keys and sets)")
	flag.Parse()

	if maxList < 0 {
		fmt.Fprintln(os.Stderr, "error: -max-list must be >= 0")
		os.Exit(2)
	}

	upstreamSHA := strings.TrimSpace(openbaoGitSHA)
	if upstreamSHA == "" {
		sha, err := resolveOpenBaoSHAFromDocker(openbaoImageTag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: resolve upstream SHA from docker image %q: %v\n", openbaoImageTag, err)
			os.Exit(1)
		}
		upstreamSHA = sha
	}

	upstreamModDir, err := materializeUpstreamModuleDir(upstreamSHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: fetch upstream openbao module at %s: %v\n", upstreamSHA, err)
		os.Exit(1)
	}

	upstreamRoots := []rootSpec{
		// Core server config fields.
		{Prefix: "", Dir: filepath.Join(upstreamModDir, "command", "server"), Type: "Config"},

		// Shared config fields (logging, telemetry block, etc).
		{Prefix: "", Dir: filepath.Join(upstreamModDir, "internalshared", "configutil"), Type: "SharedConfig"},

		// Stanzas parsed manually (listener, audit). We still extract their hcl tag keys from structs.
		{Prefix: "listener", Dir: filepath.Join(upstreamModDir, "internalshared", "configutil"), Type: "Listener"},
		{Prefix: "audit", Dir: filepath.Join(upstreamModDir, "command", "server"), Type: "AuditDevice"},
	}

	operatorRoots := []rootSpec{
		{Prefix: "", Dir: filepath.Join("internal", "config"), Type: "hclCoreAttributes"},
		{Prefix: "", Dir: filepath.Join("internal", "config"), Type: "hclUserConfigurationAttributes"},

		{Prefix: "listener", Dir: filepath.Join("internal", "config"), Type: "hclListenerTCP"},
		{Prefix: "storage", Dir: filepath.Join("internal", "config"), Type: "hclStorageRaft"},
		{Prefix: "storage.retry_join", Dir: filepath.Join("internal", "config"), Type: "hclRetryJoin"},

		{Prefix: "telemetry", Dir: filepath.Join("internal", "config"), Type: "hclTelemetry"},
		{Prefix: "audit", Dir: filepath.Join("internal", "config"), Type: "hclAuditDevice"},
		{Prefix: "plugin", Dir: filepath.Join("internal", "config"), Type: "hclPlugin"},

		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealStatic"},
		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealTransit"},
		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealAWSKMS"},
		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealAzureKeyVault"},
		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealGCPCloudKMS"},
		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealKMIP"},
		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealOCIKMS"},
		{Prefix: "seal", Dir: filepath.Join("internal", "config"), Type: "hclSealPKCS11"},
	}

	upstreamKeys, err := extractKeySet(upstreamRoots)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: extract upstream key set: %v\n", err)
		os.Exit(1)
	}

	operatorSchemaKeys, err := extractKeySet(operatorRoots)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: extract operator schema key set: %v\n", err)
		os.Exit(1)
	}
	// Stanza roots (blocks) that are rendered explicitly by the operator.
	operatorSchemaKeys["listener"] = struct{}{}
	operatorSchemaKeys["storage"] = struct{}{}
	operatorSchemaKeys["storage.retry_join"] = struct{}{}
	operatorSchemaKeys["seal"] = struct{}{}
	operatorSchemaKeys["audit"] = struct{}{}
	operatorSchemaKeys["plugin"] = struct{}{}
	operatorSchemaKeys["telemetry"] = struct{}{}
	// Operator renders audit.options as a cty object, but it's not part of the gohcl struct.
	operatorSchemaKeys["audit.options"] = struct{}{}
	// Operator always renders this empty block today (labelled), but has no typed fields.
	operatorSchemaKeys["service_registration"] = struct{}{}

	defaultGeneratedKeys, err := extractDefaultGeneratedKeySet()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: extract default generated key set: %v\n", err)
		os.Exit(1)
	}

	missingTyped := setDifference(upstreamKeys, operatorSchemaKeys)
	missingDefault := setDifference(upstreamKeys, defaultGeneratedKeys)
	operatorExtras := setDifference(operatorSchemaKeys, upstreamKeys)

	fmt.Printf("==> OpenBao config schema coverage report\n")
	fmt.Printf("    Upstream: openbao/openbao:%s (%s)\n", openbaoImageTag, upstreamSHA)
	fmt.Printf("\n")

	fmt.Printf("Upstream keys (extractable): %d\n", len(upstreamKeys))
	fmt.Printf("Operator explicit keys:      %d\n", len(operatorSchemaKeys))
	fmt.Printf("Operator default keys:       %d\n", len(defaultGeneratedKeys))
	fmt.Printf("\n")

	fmt.Printf("Upstream keys not explicitly supported by operator typed schema: %d\n", len(missingTyped))
	printKeyList("Missing (typed schema)", missingTyped, maxList)
	fmt.Printf("\n")

	fmt.Printf("Upstream keys not covered by operator default config generation: %d\n", len(missingDefault))
	printKeyList("Missing (default generation)", missingDefault, maxList)
	fmt.Printf("\n")

	fmt.Printf("Operator keys not present in upstream extracted schema: %d\n", len(operatorExtras))
	printKeyList("Extra (operator vs upstream schema)", operatorExtras, maxList)

	if verbose {
		fmt.Printf("\n-- Upstream keys (full)\n")
		printKeyList("Upstream keys", upstreamKeys, 0)
		fmt.Printf("\n-- Operator schema keys (full)\n")
		printKeyList("Operator schema keys", operatorSchemaKeys, 0)
		fmt.Printf("\n-- Operator default keys (full)\n")
		printKeyList("Operator default keys", defaultGeneratedKeys, 0)
	}

	writeGitHubSummary(openbaoImageTag, upstreamSHA, upstreamKeys, operatorSchemaKeys, defaultGeneratedKeys, missingTyped, missingDefault, operatorExtras, maxList)
}

func extractDefaultGeneratedKeySet() (map[string]struct{}, error) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "schema-report",
			Namespace: "default",
		},
	}
	infra := config.InfrastructureDetails{
		HeadlessServiceName: cluster.Name,
		Namespace:           cluster.Namespace,
		APIPort:             8200,
		ClusterPort:         8201,
	}

	hclBytes, err := config.RenderHCL(cluster, infra)
	if err != nil {
		return nil, err
	}
	parsed, diags := hclwrite.ParseConfig(hclBytes, "config.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse generated HCL: %s", diags.Error())
	}

	keys := map[string]struct{}{}
	collectKeysFromHCLBody(keys, "", parsed.Body())
	return keys, nil
}

func collectKeysFromHCLBody(out map[string]struct{}, prefix string, body *hclwrite.Body) {
	for name := range body.Attributes() {
		out[join(prefix, name)] = struct{}{}
	}
	for _, block := range body.Blocks() {
		blockName := block.Type()
		out[join(prefix, blockName)] = struct{}{}
		collectKeysFromHCLBody(out, join(prefix, blockName), block.Body())
	}
}

func extractKeySet(roots []rootSpec) (map[string]struct{}, error) {
	keys := make(map[string]struct{})
	cache := map[string]*pkgIndex{}

	for _, root := range roots {
		index, ok := cache[root.Dir]
		if !ok {
			idx, err := indexPackageDir(root.Dir)
			if err != nil {
				return nil, err
			}
			index = idx
			cache[root.Dir] = idx
		}

		st, ok := index.structs[root.Type]
		if !ok {
			return nil, fmt.Errorf("type %q not found in %s", root.Type, root.Dir)
		}
		collectKeysFromStruct(keys, index, join(root.Prefix, ""), st, map[string]bool{})
	}

	return keys, nil
}

type pkgIndex struct {
	structs map[string]*ast.StructType
	aliases map[string]ast.Expr
}

func indexPackageDir(dir string) (*pkgIndex, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info fs.FileInfo) bool {
		name := info.Name()
		if strings.HasSuffix(name, "_test.go") {
			return false
		}
		return strings.HasSuffix(name, ".go")
	}, 0)
	if err != nil {
		return nil, fmt.Errorf("parse dir %s: %w", dir, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no go packages found in %s", dir)
	}

	var first *ast.Package
	for _, p := range pkgs {
		first = p
		break
	}

	idx := &pkgIndex{
		structs: map[string]*ast.StructType{},
		aliases: map[string]ast.Expr{},
	}
	for _, f := range first.Files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				switch t := ts.Type.(type) {
				case *ast.StructType:
					idx.structs[ts.Name.Name] = t
				default:
					idx.aliases[ts.Name.Name] = ts.Type
				}
			}
		}
	}
	return idx, nil
}

func collectKeysFromStruct(out map[string]struct{}, idx *pkgIndex, prefix string, st *ast.StructType, visiting map[string]bool) {
	if st == nil || st.Fields == nil {
		return
	}

	for _, field := range st.Fields.List {
		hclName, opts, ok := parseHCLTag(field.Tag)
		if !ok {
			continue
		}
		if hclName == "" || hclName == "-" {
			continue
		}
		if opts["label"] {
			continue
		}
		if opts["unusedKeyPositions"] || opts["unusedKeys"] || opts["decodedFields"] {
			continue
		}

		full := join(prefix, hclName)
		out[full] = struct{}{}

		child, childName := resolveStructType(idx, field.Type)
		if child == nil {
			continue
		}
		if childName != "" {
			if visiting[childName] {
				continue
			}
			visiting[childName] = true
			collectKeysFromStruct(out, idx, full, child, visiting)
			delete(visiting, childName)
		} else {
			collectKeysFromStruct(out, idx, full, child, visiting)
		}
	}
}

func resolveStructType(idx *pkgIndex, expr ast.Expr) (*ast.StructType, string) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return resolveStructType(idx, t.X)
	case *ast.Ident:
		if st, ok := idx.structs[t.Name]; ok {
			return st, t.Name
		}
		alias, ok := idx.aliases[t.Name]
		if !ok {
			return nil, ""
		}
		return resolveStructType(idx, alias)
	case *ast.StructType:
		return t, ""
	default:
		return nil, ""
	}
}

func parseHCLTag(basicLit *ast.BasicLit) (name string, opts map[string]bool, ok bool) {
	if basicLit == nil || basicLit.Kind != token.STRING {
		return "", nil, false
	}
	raw, err := strconv.Unquote(basicLit.Value)
	if err != nil {
		// Fall back to trimming raw delimiters; should be rare, but keeps the tool robust.
		raw = strings.Trim(basicLit.Value, "`\"")
	}
	tag := reflectStructTag(raw)
	hclTag, ok := tag.Lookup("hcl")
	if !ok {
		return "", nil, false
	}
	parts := strings.Split(hclTag, ",")
	name = strings.TrimSpace(parts[0])
	opts = map[string]bool{}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// "alias:Foo" -> "alias"
		key := p
		if colon := strings.IndexByte(p, ':'); colon >= 0 {
			key = p[:colon]
		}
		opts[key] = true
	}
	return name, opts, true
}

func join(prefix, name string) string {
	prefix = strings.Trim(prefix, ".")
	name = strings.Trim(name, ".")
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}

func setDifference(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{})
	for k := range a {
		if _, ok := b[k]; !ok {
			out[k] = struct{}{}
		}
	}
	return out
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func printKeyList(title string, keys map[string]struct{}, max int) {
	list := sortedKeys(keys)
	if max > 0 && len(list) > max {
		list = list[:max]
	}
	if len(list) == 0 {
		fmt.Printf("%s: (none)\n", title)
		return
	}
	fmt.Printf("%s:\n", title)
	for _, k := range list {
		fmt.Printf("  - %s\n", k)
	}
}

func resolveOpenBaoSHAFromDocker(imageTag string) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker not found in PATH")
	}
	out, err := exec.Command("docker", "run", "--rm", "openbao/openbao:"+imageTag, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	line := strings.TrimSpace(string(out))
	open := strings.IndexByte(line, '(')
	close := strings.IndexByte(line, ')')
	if open < 0 || close < 0 || close <= open+1 {
		return "", fmt.Errorf("unexpected version output: %q", line)
	}
	sha := line[open+1 : close]
	if len(sha) != 40 {
		return "", fmt.Errorf("unexpected git sha %q from output %q", sha, line)
	}
	return sha, nil
}

func materializeUpstreamModuleDir(sha string) (string, error) {
	tmpdir, err := os.MkdirTemp("", "openbao-schema-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpdir)

	mainModule := filepath.Join(tmpdir, "mod")
	if err := os.MkdirAll(mainModule, 0o755); err != nil {
		return "", err
	}

	cmd := exec.Command("go", "mod", "init", "tmp.example/openbao-schema")
	cmd.Dir = mainModule
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go mod init: %v: %s", err, strings.TrimSpace(string(out)))
	}

	cmd = exec.Command("go", "get", "github.com/openbao/openbao@"+sha)
	cmd.Dir = mainModule
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go get: %v: %s", err, strings.TrimSpace(string(out)))
	}

	cmd = exec.Command("go", "list", "-m", "-json", "github.com/openbao/openbao")
	cmd.Dir = mainModule
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go list -m: %v: %s", err, strings.TrimSpace(string(out)))
	}
	var mod struct {
		Dir string `json:"Dir"`
	}
	if err := json.Unmarshal(out, &mod); err != nil {
		return "", fmt.Errorf("decode go list -m output: %w", err)
	}
	if strings.TrimSpace(mod.Dir) == "" {
		return "", fmt.Errorf("go list -m did not return .Dir")
	}
	return mod.Dir, nil
}

func writeGitHubSummary(openbaoImageTag, upstreamSHA string, upstreamKeys, operatorSchemaKeys, defaultKeys, missingTyped, missingDefault, extras map[string]struct{}, maxList int) {
	path := strings.TrimSpace(os.Getenv("GITHUB_STEP_SUMMARY"))
	if path == "" {
		return
	}

	var buf bytes.Buffer
	buf.WriteString("### OpenBao operator schema drift\n\n")
	buf.WriteString(fmt.Sprintf("- Upstream: `openbao/openbao:%s` (`%s`)\n", openbaoImageTag, upstreamSHA))
	buf.WriteString(fmt.Sprintf("- Upstream extractable keys: `%d`\n", len(upstreamKeys)))
	buf.WriteString(fmt.Sprintf("- Operator explicit keys: `%d`\n", len(operatorSchemaKeys)))
	buf.WriteString(fmt.Sprintf("- Operator default keys: `%d`\n", len(defaultKeys)))
	buf.WriteString(fmt.Sprintf("- Missing vs typed schema: `%d`\n", len(missingTyped)))
	buf.WriteString(fmt.Sprintf("- Missing vs default generation: `%d`\n", len(missingDefault)))
	buf.WriteString(fmt.Sprintf("- Operator extras vs upstream: `%d`\n", len(extras)))

	writeList := func(title string, set map[string]struct{}) {
		buf.WriteString("\n")
		buf.WriteString("#### " + title + "\n\n")
		list := sortedKeys(set)
		if maxList > 0 && len(list) > maxList {
			list = list[:maxList]
		}
		if len(list) == 0 {
			buf.WriteString("_None_\n")
			return
		}
		for _, k := range list {
			buf.WriteString("- `" + k + "`\n")
		}
	}

	writeList("Missing (typed schema)", missingTyped)
	writeList("Missing (default generation)", missingDefault)
	writeList("Extra (operator vs upstream)", extras)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(buf.Bytes())
}

// reflectStructTag is a tiny subset of reflect.StructTag to avoid importing reflect
// (and to keep behavior predictable for our parsing needs).
type reflectStructTag string

func (tag reflectStructTag) Lookup(key string) (string, bool) {
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
