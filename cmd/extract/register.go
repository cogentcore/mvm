package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/importer"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// goparserPath is the import path of the goparser package, which the
// generated files use to register generic shims.
const goparserPath = "github.com/cogentcore/mvm/goparser"

// regPkg is a package loaded for the -register mode: its syntax and types.
type regPkg struct {
	path  string
	name  string
	fset  *token.FileSet
	files []*ast.File
	types *types.Package
	info  *types.Info
}

// listPkg is the part of the 'go list -json' output of a package that
// loadPackages uses.
type listPkg struct {
	ImportPath string
	Name       string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
	Export     string
	ImportMap  map[string]string
	DepOnly    bool
	Error      *struct{ Err string }
}

// runRegister writes the symbols of each package of paths to a file of its
// own, gen_<import-path>.go in the current directory, of package pkgName.
// Each file registers a loader with register that adds the values of the
// exported names of the package with add, and registers the generic
// functions of the package as goparser generic shims, whose source mvm
// interprets. The package of the files must define register and add:
//
//	func register(loader func()) bool
//	func add(pkgs map[string]map[string]reflect.Value)
//
// so that it can add the symbols when they are first needed.
func runRegister(pkgName string, paths []string) error {
	pkgs, err := loadPackages(paths)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		src, err := registerFile(pkg, pkgName)
		if err != nil {
			return fmt.Errorf("%s: %w", pkg.path, err)
		}
		name := "gen_" + strings.NewReplacer("/", "_", ".", "_").Replace(pkg.path) + ".go"
		if err := os.WriteFile(name, src, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// loadPackages type-checks the packages of paths from their source,
// importing their dependencies from the export data that 'go list -export'
// builds.
func loadPackages(paths []string) ([]*regPkg, error) {
	args := append([]string{"list", "-e", "-export", "-deps", "-json=ImportPath,Name,Dir,GoFiles,CgoFiles,Export,ImportMap,DepOnly,Error"}, paths...)
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	// The export data of every package, and the packages of paths.
	exports := map[string]string{}
	var targets []*listPkg
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		lp := &listPkg{}
		if err := dec.Decode(lp); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		if lp.Error != nil {
			return nil, fmt.Errorf("%s: %s", lp.ImportPath, lp.Error.Err)
		}
		exports[lp.ImportPath] = lp.Export
		if !lp.DepOnly {
			targets = append(targets, lp)
		}
	}

	var pkgs []*regPkg
	for _, lp := range targets {
		pkg, err := checkPackage(lp, exports)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

// checkPackage parses and type-checks the package lp, importing its
// dependencies from their export data in exports.
func checkPackage(lp *listPkg, exports map[string]string) (*regPkg, error) {
	pkg := &regPkg{path: lp.ImportPath, name: lp.Name, fset: token.NewFileSet()}
	for _, name := range append(slices.Clone(lp.GoFiles), lp.CgoFiles...) {
		f, err := parser.ParseFile(pkg.fset, filepath.Join(lp.Dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		pkg.files = append(pkg.files, f)
	}

	// Imports are resolved through the import map of the package (vendoring)
	// to the export data of the package imported.
	lookup := func(path string) (io.ReadCloser, error) {
		if p, ok := lp.ImportMap[path]; ok {
			path = p
		}
		file := exports[path]
		if file == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	}
	conf := types.Config{Importer: importer.ForCompiler(pkg.fset, "gc", lookup), FakeImportC: true}
	pkg.info = &types.Info{Uses: map[*ast.Ident]types.Object{}}
	tp, err := conf.Check(pkg.path, pkg.fset, pkg.files, pkg.info)
	if err != nil {
		return nil, err
	}
	pkg.types = tp
	return pkg, nil
}

// registerFile returns the symbols file of pkg, of package pkgName.
func registerFile(pkg *regPkg, pkgName string) ([]byte, error) {
	scope := pkg.types.Scope()
	alias := "p_" + strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(pkg.path)

	// The values of the exported names, and the generic functions and the
	// constraints, which are shim source.
	var values []string
	bound := map[string]bool{}
	var generics []string
	constraints := map[string]bool{}
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		q := alias + "." + name
		nvalues := len(values)
		switch obj := obj.(type) {
		case *types.Func:
			if obj.Type().(*types.Signature).TypeParams().Len() > 0 {
				generics = append(generics, name)
				continue
			}
			values = append(values, fmt.Sprintf("%q: reflect.ValueOf(%s),", name, q))
		case *types.Var:
			values = append(values, fmt.Sprintf("%q: reflect.ValueOf(&%s),", name, q))
		case *types.Const:
			values = append(values, fmt.Sprintf("%q: reflect.ValueOf(%s),", name, constValueExpr(obj, q)))
		case *types.TypeName:
			if obj.IsAlias() {
				if _, ok := obj.Type().(*types.Basic); ok {
					continue
				}
			}
			// Generic types are not supported, but instances of them, which
			// an alias can name, are.
			if named, ok := obj.Type().(*types.Named); ok && named.TypeParams().Len() > 0 && named.TypeArgs().Len() == 0 {
				continue
			}
			if iface, ok := obj.Type().Underlying().(*types.Interface); ok && !iface.IsMethodSet() {
				constraints[name] = true
				continue
			}
			values = append(values, fmt.Sprintf("%q: reflect.ValueOf((*%s)(nil)),", name, q))
		}
		if len(values) > nvalues {
			bound[name] = true
		}
	}

	shim, refs, err := shimSource(pkg, generics, constraints, bound)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by mvm extract -register %s. DO NOT EDIT.\n\npackage %s\n\nimport (\n", pkg.path, pkgName)
	if len(values) > 0 {
		fmt.Fprintf(&b, "\t%s %q\n", alias, pkg.path)
	}
	b.WriteString("\t\"reflect\"\n")
	if shim != "" {
		fmt.Fprintf(&b, "\t%q\n", goparserPath)
	}
	b.WriteString(")\n\nvar _ = register(func() {\n")
	if shim != "" {
		fmt.Fprintf(&b, "goparser.RegisterGenericShim(%q, %s, %#v)\n", pkg.path, backquote(shim), refs)
	}
	fmt.Fprintf(&b, "add(map[string]map[string]reflect.Value{\n%q: {\n", pkg.path)
	for _, v := range values {
		b.WriteString(v + "\n")
	}
	b.WriteString("},\n})\n})\n")
	return format.Source(b.Bytes())
}

// constValueExpr returns the expression of the constant obj, named by q: q,
// converted to uint64 if it is an untyped integer too large for an int.
func constValueExpr(obj *types.Const, q string) string {
	if b, ok := obj.Type().(*types.Basic); ok && b.Info()&types.IsUntyped != 0 && obj.Val().Kind() == constant.Int {
		if _, exact := constant.Int64Val(obj.Val()); !exact {
			return "uint64(" + q + ")"
		}
	}
	return q
}

// backquote returns s as a Go raw string literal, or a quoted string if it
// has a backquote.
func backquote(s string) string {
	if strings.Contains(s, "`") {
		return fmt.Sprintf("%q", s)
	}
	return "`" + s + "`"
}

// shimSource returns the source of the shim of the generic functions of pkg
// named generics, with the declarations of the constraints that they use,
// and the names of the package that the source refers to, which the shim
// gets from the values of the package, bound. A generic function that refers
// to a name of the package that is neither bound nor in the shim, such as an
// unexported name or a generic type, is left out, as it could not be
// resolved.
func shimSource(pkg *regPkg, generics []string, constraints, bound map[string]bool) (string, []string, error) {
	if len(generics) == 0 {
		return "", nil, nil
	}
	decls := map[string]ast.Decl{}
	for _, f := range pkg.files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && slices.Contains(generics, d.Name.Name) {
					decls[d.Name.Name] = d
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok && constraints[ts.Name.Name] {
						decls[ts.Name.Name] = &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{ts}}
					}
				}
			}
		}
	}

	// Find what each declaration refers to: names of the package and
	// imported packages.
	scope := pkg.types.Scope()
	type uses struct {
		names   map[string]bool
		imports map[string]string // import path by package name
	}
	use := func(d ast.Decl) uses {
		u := uses{names: map[string]bool{}, imports: map[string]string{}}
		ast.Inspect(d, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			switch obj := pkg.info.Uses[id].(type) {
			case *types.PkgName:
				u.imports[id.Name] = obj.Imported().Path()
			case nil:
			default:
				if obj.Pkg() == pkg.types && obj.Parent() == scope {
					u.names[obj.Name()] = true
				}
			}
			return true
		})
		return u
	}
	var names []string
	declUses := map[string]uses{}
	for name, d := range decls {
		names = append(names, name)
		declUses[name] = use(d)
	}
	slices.Sort(names)

	// Leave out the declarations that refer to names that are neither bound
	// nor kept, until all of those kept refer only to such names, as leaving
	// one out can leave out those that refer to it.
	for changed := true; changed; {
		changed = false
		for _, name := range names {
			if decls[name] == nil {
				continue
			}
			for n := range declUses[name].names {
				if n != name && !bound[n] && decls[n] == nil {
					log.Printf("%s: leaving out %s, which uses %s, which is not bound", pkg.path, name, n)
					delete(decls, name)
					changed = true
					break
				}
			}
		}
	}

	// The generic functions and constraints kept, and the names and imports
	// that they use.
	var srcs []string
	refs := map[string]bool{}
	imports := map[string]string{}
	for _, name := range names {
		d := decls[name]
		if d == nil {
			continue
		}
		u := declUses[name]
		var b bytes.Buffer
		if fd, ok := d.(*ast.FuncDecl); ok {
			fd.Doc = nil
		}
		if err := printer.Fprint(&b, pkg.fset, d); err != nil {
			return "", nil, err
		}
		srcs = append(srcs, stripLineComments(b.String()))
		for n := range u.names {
			if n != name && decls[n] == nil {
				refs[n] = true
			}
		}
		for k, v := range u.imports {
			imports[k] = v
		}
	}
	if len(srcs) == 0 {
		return "", nil, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\n", pkg.name)
	var inames []string
	for k := range imports {
		inames = append(inames, k)
	}
	slices.Sort(inames)
	for _, k := range inames {
		fmt.Fprintf(&b, "import %s %q\n", k, imports[k])
	}
	for _, s := range srcs {
		b.WriteString("\n" + s + "\n")
	}
	var rl []string
	for k := range refs {
		rl = append(rl, k)
	}
	slices.Sort(rl)
	return b.String(), rl, nil
}

// stripLineComments removes the line comments of src that are on lines of
// their own.
func stripLineComments(src string) string {
	var out []string
	for _, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
