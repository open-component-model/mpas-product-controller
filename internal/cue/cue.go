package cue

import (
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"golang.org/x/exp/slices"
)

const (
	privateAttr = "private"
)

var defaultOpts = []cue.Option{
	cue.All(),
	cue.Docs(true),
	cue.Hidden(false),
}

// File is a wrapper around a cue file.
// It provides methods for working with cue files.
type File struct {
	Name          string
	ctx           *cue.Context
	file          *ast.File
	v             cue.Value
	schemaVersion string
}

// New creates a new File from a cue file.
func New(name, filePath string) (*File, error) {
	ctx := cuecontext.New()
	file, err := parseFile(ctx, filePath)
	if err != nil {
		return nil, err
	}
	return &File{
		Name: name,
		ctx:  ctx,
		file: file,
	}, nil
}

func (f *File) SchemaVersion() (string, error) {
	if f.schemaVersion != "" {
		return f.schemaVersion, nil
	}

	var err error
	f.schemaVersion, err = f.value().LookupPath(cue.ParsePath("#SchemaVersion")).String()
	if err != nil {
		return "", err
	}
	return f.schemaVersion, nil
}

func (f *File) Comments() string {
	var comments string
	for _, s := range f.file.Comments() {
		comments += s.Text()
	}
	return comments
}

func (f *File) value() cue.Value {
	if f.v.Exists() {
		return f.v
	}
	v := f.ctx.BuildFile(f.file)
	f.v = v
	return f.v
}

func (f *File) deltaFrom(file *ast.File) cue.Value {
	return f.ctx.BuildFile(file, cue.Scope(f.ctx.BuildFile(f.file)))
}

func (f *File) Merge(schema *File, parents ...cue.Selector) (*File, error) {
	sv := schema.value()
	dv := f.value()
	fields, err := fieldsDelta(sv, dv, parents...)
	if err != nil {
		return nil, err
	}

	schemaVersion, err := schema.SchemaVersion()
	if err != nil {
		return nil, err
	}

	decls, err := generateDefaults(sv, fields, schemaVersion)
	if err != nil {
		return nil, err
	}

	completed := &ast.File{
		Decls: decls,
	}

	completed.SetComments(schema.file.Comments())
	delta := f.deltaFrom(completed)
	v := unify(dv, delta)

	return Export(f.Name, v), nil
}

func (f *File) Sanitize() error {
	return astutil.Sanitize(f.file)
}

func (f *File) Unify(files []*File) *File {
	v := f.value()
	for _, file := range files {
		v = unify(v, file.value())
	}
	return Export(f.Name, v)
}

func unify(v1, v2 cue.Value) cue.Value {
	return v1.Unify(v2)
}

func (f *File) Json() ([]byte, error) {
	return f.value().MarshalJSON()
}

func (f *File) Format() ([]byte, error) {
	return format.Node(f.file, format.Simplify())
}

func Export(name string, v cue.Value, opts ...cue.Option) *File {
	opts = append(defaultOpts, opts...)
	ctx := cuecontext.New()
	file := &File{
		Name: name,
		ctx:  ctx,
		file: &ast.File{
			Decls: v.Syntax(opts...).(*ast.StructLit).Elts,
		},
	}
	return file
}

func parseFile(ctx *cue.Context, p string) (*ast.File, error) {
	tree, err := parser.ParseFile(p, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func fieldsDelta(schema, data cue.Value, parents ...cue.Selector) ([]cue.Path, error) {
	m := make([]cue.Path, 0)
	i, err := schema.Fields(defaultOpts...)
	if err != nil {
		return nil, err
	}

	for i.Next() {
		sel := append(parents, i.Selector())
		path := cue.MakePath(sel...)

		// skip private fields based on the private attribute
		// e.g. @private(true)
		attr := i.Value().Attribute(privateAttr)
		if err := attr.Err(); err == nil {
			continue
		}

		entry := data.LookupPath(path)
		if !entry.Exists() {
			m = append(m, path)
		}

		switch i.Value().Syntax().(type) {
		case *ast.StructLit:
			// recurse into the struct
			// to find missing fields
			x := schema.LookupPath(path)
			n, err := fieldsDelta(x, data, sel[1:]...)
			if err != nil {
				return nil, err
			}

			// restore the selector prefix to the path
			for _, nv := range n {
				nsel := append(sel[:], nv.Selectors()...)
				m = append(m, cue.MakePath(nsel...))
			}
		}
	}

	return m, nil
}

func generateDefaults(input cue.Value, fields []cue.Path, schemaVersion string) ([]ast.Decl, error) {
	f, err := input.Fields(defaultOpts...)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, p := range fields {
		paths = append(paths, p.String())
	}

	return makeValues(f, paths, schemaVersion)
}

func makeValues(i *cue.Iterator, paths []string, schemaVersion string, parents ...cue.Selector) ([]ast.Decl, error) {
	result := make([]ast.Decl, 0)
	for i.Next() {
		var v ast.Expr
		value := i.Value()
		sel := append(parents, i.Selector())
		path := cue.MakePath(sel...)

		if !slices.Contains(paths, path.String()) {
			continue
		}

		field, hasDefaultValue := value.Default()

		if !hasDefaultValue && value.IsConcrete() {
			switch value.Syntax(cue.Raw()).(type) {
			case *ast.StructLit:
				var rx []ast.Decl
				f, err := value.Fields()
				if err != nil {
					return nil, err
				}
				rx, err = makeValues(f, paths, schemaVersion, sel...)
				if err != nil {
					return nil, err
				}
				v = &ast.StructLit{
					Elts: rx,
				}
			default:
				v = field.Syntax(cue.Raw()).(ast.Expr)
			}
		} else {
			v = field.Syntax(cue.Raw()).(ast.Expr)
		}

		label, _ := value.Label()
		f := &ast.Field{
			Label: ast.NewIdent(label),
		}

		if v != nil {
			f.Value = v
		} else {
			switch vnode := value.Syntax(cue.Raw()).(type) {
			case *ast.BinaryExpr:
				f.Value = vnode
			default:
				f.Value = ast.NewIdent(value.IncompleteKind().String())
			}
		}

		for _, av := range value.Attributes(cue.ValueAttr) {
			ax := &ast.Attribute{
				Text: fmt.Sprintf(
					"@%s(%s,schema_version=\"%s\")",
					av.Name(),
					av.Contents(),
					schemaVersion,
				),
			}
			f.Attrs = append(f.Attrs, ax)
		}
		ast.SetComments(f, value.Doc())
		result = append(result, f)
	}
	return result, nil
}
