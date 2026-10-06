package langc

import (
	"bytes"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"webtyp.com/modfind"
)

// Import paths whose identifiers the rules recognise.
const (
	pathLang  = "webtyp.com/lang"
	pathFmt   = "webtyp.com/fmt"
	pathModel = "webtyp.com/model"
	pathInput = "webtyp.com/input"
)

// clientBuild is the build context of the browser binary.
var clientBuild = func() build.Context {
	c := build.Default
	c.GOOS, c.GOARCH = "js", "wasm"
	c.CgoEnabled = false
	return c
}()

// skipDirs never hold code that reaches a build.
var skipDirs = []string{"vendor", "node_modules", "testdata", "_temp"}

// prefilter: a file that imports no webtyp package cannot use any rule.
var prefilter = []byte(`"webtyp.com/`)

// keyUses maps each key to the module paths that use it.
type keyUses map[string]map[string]bool

func (u keyUses) add(key, module string) {
	if !hasLetter(key) {
		return
	}
	if u[key] == nil {
		u[key] = map[string]bool{}
	}
	u[key][module] = true
}

// hasLetter reports whether s has a letter outside format verbs: "%s?" and
// "192.168.1.1" are not translatable text.
func hasLetter(s string) bool {
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '%' {
			// skip flags, width and precision, then the verb letter
			for i++; i < len(rs) && !unicode.IsLetter(rs[i]) && rs[i] != '%'; i++ {
			}
			continue
		}
		if unicode.IsLetter(rs[i]) {
			return true
		}
	}
	return false
}

// srcFile is one parsed Go file with what the rules need to resolve names.
type srcFile struct {
	module  string // module path, recorded as the user of each key
	pkgPath string // import path of the file's package
	file    *ast.File
	imports map[string]string // local name → import path
	dots    map[string]bool   // dot-imported paths
	consts  constValues       // package-level string constants of every scanned file
}

// constValues maps "importPath.Name" to the value of a package-level string
// constant, so a key passed as a named constant (the ecosystem's rule for
// repeated strings) is found like a literal.
type constValues map[string]string

// collectConsts records every package-level `const X = "..."` (typed or not).
func collectConsts(files []*srcFile) constValues {
	cv := constValues{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if i < len(vs.Values) {
						if v, ok := stringLit(vs.Values[i]); ok {
							cv[f.pkgPath+"."+n.Name] = v
						}
					}
				}
			}
		}
	}
	for _, f := range files {
		f.consts = cv
	}
	return cv
}

// textArg returns the text of an argument: a string literal, or a named
// string constant of this package or an imported one.
func (f *srcFile) textArg(e ast.Expr) (string, bool) {
	if s, ok := stringLit(e); ok {
		return s, true
	}
	switch e.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		if p, n, ok := f.resolve(e, ""); ok {
			v, ok := f.consts[p+"."+n]
			return v, ok
		}
	}
	return "", false
}

// resolve returns the import path and name an expression refers to:
// pkg.Name through the import block, a bare Name inside its own package or
// through a dot import (dotPath is the path the caller expects).
func (f *srcFile) resolve(e ast.Expr, dotPath string) (string, string, bool) {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		id, ok := x.X.(*ast.Ident)
		if !ok {
			return "", "", false
		}
		p, ok := f.imports[id.Name]
		return p, x.Sel.Name, ok
	case *ast.Ident:
		if dotPath != "" && (f.pkgPath == dotPath || f.dots[dotPath]) {
			return dotPath, x.Name, true
		}
		return f.pkgPath, x.Name, true
	case *ast.StarExpr:
		return f.resolve(x.X, dotPath)
	}
	return "", "", false
}

func (f *srcFile) is(e ast.Expr, path, name string) bool {
	p, n, ok := f.resolve(e, path)
	return ok && p == path && n == name
}

func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}

// parseModule parses every non-test Go file of a module, skipping hidden
// directories, skipDirs and nested modules.
func parseModule(m modfind.Module, ctx build.Context) []*srcFile {
	root := m.SourceDir()
	if root == "" {
		return nil
	}
	var files []*srcFile
	fset := token.NewFileSet()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || contains(skipDirs, name) {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir // nested module
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if ok, err := ctx.MatchFile(filepath.Dir(path), d.Name()); err != nil || !ok {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(src, prefilter) {
			return nil
		}
		af, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkgPath := m.Path
		if rel != "." {
			pkgPath += "/" + filepath.ToSlash(rel)
		}
		files = append(files, newSrcFile(m.Path, pkgPath, af))
		return nil
	})
	return files
}

func newSrcFile(module, pkgPath string, af *ast.File) *srcFile {
	f := &srcFile{module: module, pkgPath: pkgPath, file: af, imports: map[string]string{}, dots: map[string]bool{}}
	for _, is := range af.Imports {
		p, err := strconv.Unquote(is.Path.Value)
		if err != nil {
			continue
		}
		switch {
		case is.Name == nil:
			f.imports[p[strings.LastIndex(p, "/")+1:]] = p
		case is.Name.Name == ".":
			f.dots[p] = true
		case is.Name.Name != "_":
			f.imports[is.Name.Name] = p
		}
	}
	return f
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// textFields records, per "pkgPath.TypeName", the struct fields typed lang.Text.
type textFields map[string]map[string]bool

// collectTextFields is rule 7, pass 1.
func collectTextFields(files []*srcFile) textFields {
	tf := textFields{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, fld := range st.Fields.List {
					if !f.is(fld.Type, pathLang, "Text") {
						continue
					}
					key := f.pkgPath + "." + ts.Name.Name
					if tf[key] == nil {
						tf[key] = map[string]bool{}
					}
					for _, n := range fld.Names {
						tf[key][n.Name] = true
					}
				}
			}
		}
	}
	return tf
}

// collectKeys applies rules 1–8 to every file.
func collectKeys(files []*srcFile, tf textFields, rule2Scope map[string]bool) keyUses {
	collectConsts(files)
	uses := keyUses{}
	for _, f := range files {
		f := f
		ast.Inspect(f.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				f.callKeys(x, uses, rule2Scope)
			case *ast.CompositeLit:
				f.literalKeys(x, tf, uses)
			case *ast.FuncDecl:
				f.textReturns(x, uses)
			}
			return true
		})
	}
	return uses
}

func (f *srcFile) callKeys(call *ast.CallExpr, uses keyUses, rule2Scope map[string]bool) {
	// Rule 1: lang.Translate("...", ...). Rule 2: fmt.Err("...", ...).
	// Rule 7 (conversions): lang.Text("...").

	if f.is(call.Fun, pathLang, "Translate") || f.is(call.Fun, pathLang, "Text") {
		for _, a := range call.Args {
			if s, ok := f.textArg(a); ok {
				uses.add(s, f.module)
			}
		}
		return
	}

	if f.is(call.Fun, pathFmt, "Err") {
		if rule2Scope != nil && !rule2Scope[f.module] {
			return
		}
		for _, a := range call.Args {
			if s, ok := f.textArg(a); ok {
				uses.add(s, f.module)
			}
		}
		return
	}
	// Rule 4: fmt.KeyValue{Value: "..."} passed directly to a webtyp.com/input function.
	if p, _, ok := f.resolve(call.Fun, ""); ok && p == pathInput {
		for _, a := range call.Args {
			cl, ok := a.(*ast.CompositeLit)
			if !ok || cl.Type == nil || !f.is(cl.Type, pathFmt, "KeyValue") {
				continue
			}
			if s, ok := fieldLit(cl, "Value"); ok {
				uses.add(s, f.module)
			}
		}
	}
	// Rule 5: inside webtyp.com/input, SetPlaceholder/SetTitle string arguments.
	if f.module == pathInput {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "SetPlaceholder" || sel.Sel.Name == "SetTitle") {
			for _, a := range call.Args {
				if s, ok := f.textArg(a); ok {
					uses.add(s, f.module)
				}
			}
		}
	}
}

func (f *srcFile) literalKeys(cl *ast.CompositeLit, tf textFields, uses keyUses) {
	if cl.Type == nil {
		return // element of an outer literal: handled with the outer type below
	}
	// Rules 3 and 6: model.Field literals, direct or as elements of
	// model.Fields{...} / []model.Field{...}.
	if f.is(cl.Type, pathModel, "Field") {
		f.fieldKeys(cl, uses)
	}
	if elems := f.elementsOf(cl, pathModel, "Field"); elems != nil || f.is(cl.Type, pathModel, "Fields") {
		if elems == nil {
			elems = cl.Elts
		}
		for _, e := range elems {
			if el, ok := e.(*ast.CompositeLit); ok && el.Type == nil {
				f.fieldKeys(el, uses)
			}
		}
	}
	// Rule 7, pass 2: literals of a type with lang.Text fields, direct or as
	// type-elided elements of a slice of that type.
	if fields := f.textFieldsOf(cl.Type, tf); fields != nil {
		f.textLitKeys(cl, fields, uses)
	}
	if at, ok := cl.Type.(*ast.ArrayType); ok {
		if fields := f.textFieldsOf(at.Elt, tf); fields != nil {
			for _, e := range cl.Elts {
				if el, ok := e.(*ast.CompositeLit); ok && el.Type == nil {
					f.textLitKeys(el, fields, uses)
				}
			}
		}
	}
}

// elementsOf returns cl's elements when cl is a []path.name literal.
func (f *srcFile) elementsOf(cl *ast.CompositeLit, path, name string) []ast.Expr {
	at, ok := cl.Type.(*ast.ArrayType)
	if !ok || !f.is(at.Elt, path, name) {
		return nil
	}
	return cl.Elts
}

func (f *srcFile) fieldKeys(cl *ast.CompositeLit, uses keyUses) {
	label, hasLabel := fieldLit(cl, "Label")
	if hasLabel {
		uses.add(label, f.module)
	} else if name, ok := fieldLit(cl, "Name"); ok {
		uses.add(strings.ReplaceAll(name, "_", " "), f.module) // what form shows: "is_active" → "is active"
	}
	if help, ok := fieldLit(cl, "Help"); ok {
		uses.add(help, f.module)
	}
}

func (f *srcFile) textFieldsOf(t ast.Expr, tf textFields) map[string]bool {
	p, n, ok := f.resolve(t, "")
	if !ok {
		return nil
	}
	return tf[p+"."+n]
}

func (f *srcFile) textLitKeys(cl *ast.CompositeLit, fields map[string]bool, uses keyUses) {
	for _, e := range cl.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if !ok || !fields[id.Name] {
			continue
		}
		if s, ok := stringLit(kv.Value); ok {
			uses.add(s, f.module)
		}
	}
}

// textReturns is rule 8: string literals returned by a func whose single
// result is lang.Text.
func (f *srcFile) textReturns(fd *ast.FuncDecl, uses keyUses) {
	if fd.Body == nil || fd.Type.Results == nil || len(fd.Type.Results.List) != 1 {
		return
	}
	r := fd.Type.Results.List[0]
	if len(r.Names) > 1 || !f.is(r.Type, pathLang, "Text") {
		return
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false // a nested func has its own results
		}
		if rs, ok := n.(*ast.ReturnStmt); ok && len(rs.Results) == 1 {
			if s, ok := stringLit(rs.Results[0]); ok {
				uses.add(s, f.module)
			}
		}
		return true
	})
}

// fieldLit returns the string literal assigned to key in a keyed literal.
func fieldLit(cl *ast.CompositeLit, key string) (string, bool) {
	for _, e := range cl.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
			return stringLit(kv.Value)
		}
	}
	return "", false
}
