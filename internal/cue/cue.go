// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package cue

import (
	"bytes"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/pkg/encoding/json"
	"cuelang.org/go/pkg/encoding/yaml"
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
	schemaVersion string
	ctx           *cue.Context
	file          *ast.File
	v             cue.Value
}

// New creates a new File from a cue file.
// src must be a string, []byte, or io.Reader.
// if src is nil, the file is read from filepath.
func New(name, filepath string, src any) (*File, error) {
	ctx := cuecontext.New()
	file, err := parse(ctx, filepath, src)
	if err != nil {
		return nil, fmt.Errorf(fmt.Errorf("failed to parse cue file: %w", err).Error())
	}
	return &File{
		Name: name,
		ctx:  ctx,
		file: file,
		v:    ctx.BuildFile(file),
	}, nil
}

// SchemaVersion returns the schema version of the cue file.
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

// Comments returns the comments of the top Node of the cue file.
// It returns an empty string if there are no comments.
func (f *File) Comments() string {
	var comments string
	for _, s := range f.file.Comments() {
		comments += s.Text()
	}
	return comments
}

func (f *File) setComments(cgs []*ast.CommentGroup) {
	f.file.SetComments(cgs)
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

// CopyWithoutPrivateFields returns a copy of the cue file without private fields.
// note: calling Eval() after CopyWithoutPrivateFields() will add the private fields back.
func (f *File) EvalWithoutPrivateFields() *File {
	ctx := cuecontext.New()
	file := &File{
		Name: f.Name,
		ctx:  ctx,
		file: &ast.File{
			Decls: f.file.Decls,
		},
		v: f.v,
	}

	syn := []cue.Option{
		cue.Final(),
		cue.Definitions(true),
		cue.Attributes(true),
		cue.Optional(false),
		cue.ErrorsAsValues(false),
	}

	file = eval(f, syn)

	removePrivateFields(&file.file.Decls)
	f.v = ctx.BuildFile(file.file)
	file.setComments(f.v.Doc())
	return file
}

// ContainsPrivateFields returns true if the cue file contains private fields.
func (f *File) ContainsPrivateFields() bool {
	return containsPrivateFields(f.file.Decls)
}

func containsPrivateFields(values []ast.Decl) bool {
	for _, decl := range values {
		if f, ok := decl.(*ast.Field); ok {
			if len(f.Attrs) > 0 && f.Attrs[0].Text == "@private(true)" {
				return true
			}
			if val, ok := f.Value.(*ast.StructLit); ok {
				return containsPrivateFields(val.Elts)
			}
		}
	}
	return false
}

func removePrivateFields(values *[]ast.Decl) {
	for i, decl := range *values {
		if f, ok := decl.(*ast.Field); ok {
			if len(f.Attrs) > 0 && f.Attrs[0].Text == "@private(true)" {
				*values = append((*values)[:i], (*values)[i+1:]...)
			}
			if val, ok := f.Value.(*ast.StructLit); ok {
				removePrivateFields(&val.Elts)
			}
		}
	}
}

// Merge merges the schema with the data.
// It strip any private fields from the schema.
func (f *File) Merge(schema *File, parents ...cue.Selector) (*File, error) {
	sv := schema.value()
	dv := f.value()

	fields, err := fieldsDelta(sv, dv, defaultOpts, parents...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate delta: %w", err)
	}

	schemaVersion, err := schema.SchemaVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get schema version: %w", err)
	}

	decls, err := generateDefaults(sv, fields, schemaVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to generate defaults: %w", err)
	}

	completed := &ast.File{
		Decls: decls,
	}

	delta := f.deltaFrom(completed)
	v := unify(dv, delta)

	err = v.Validate(defaultOpts...)
	if err != nil {
		return nil, err
	}

	return Export(f.Name, v), nil
}

// Sanitize makes sure the cue file is well formed.
func (f *File) Sanitize() error {
	return astutil.Sanitize(f.file)
}

// Validate validates the cue file against the given schema.
func (f *File) Validate(schema *File) error {
	unified, err := f.Unify([]*File{schema})
	if err != nil {
		return err
	}
	return unified.Vet()
}

// Unify merges several cue files into one.
// Unify reports the greatest lower bound of the given files.
func (f *File) Unify(files []*File) (*File, error) {
	v := f.value()
	for _, file := range files {
		v = unify(v, file.value())
	}
	opts := append(defaultOpts, cue.Concrete(false))
	err := v.Validate(opts...)
	if err != nil {
		return nil, err
	}
	return Export(f.Name, v), nil
}

func unify(v1, v2 cue.Value) cue.Value {
	return v1.Unify(v2)
}

// Json returns the json representation of the cue file.
func (f *File) Json() (string, error) {
	return json.Marshal(f.value())
}

// Format formats the cue file.
func (f *File) Format() ([]byte, error) {
	return format.Node(f.file, format.Simplify())
}

// Yaml returns the yaml representation of the cue file.
func (f *File) Yaml() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString("---\n")
	str, err := yaml.Marshal(f.value())
	if err != nil {
		return nil, err
	}
	_, err = buf.WriteString(str)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Eval evaluates the cue file.
// It expects the cue file to be well formed.
// Optional fields and attributes are removed.
func (f *File) Eval() *File {
	syn := []cue.Option{
		cue.Final(),
		cue.Definitions(true),
		cue.Attributes(false),
		cue.Optional(false),
		cue.ErrorsAsValues(false),
	}

	return eval(f, syn)
}

func eval(f *File, syn []cue.Option) *File {
	v := f.value()
	node := v.Syntax(syn...)
	newfile := &ast.File{
		Filename: f.file.Filename,
		Decls:    node.(*ast.StructLit).Elts,
	}
	f.file = newfile
	f.v = f.ctx.BuildFile(newfile)
	f.setComments(v.Doc())

	return f
}

// Vet validates the cue file.
// It expects the cue file to be well formed and with concrete values.
func (f *File) Vet() error {
	opt := []cue.Option{
		cue.Final(),
	}

	iter, err := f.v.Fields(opt...)
	if err != nil {
		return err
	}

	for iter.Next() {
		v := iter.Value()
		err := v.Validate(append(opt, cue.Concrete(true))...)
		if err != nil {
			return fmt.Errorf("failed: %w", err)
		}
	}
	return nil
}

// Export exports the cue value to a cue file.
func Export(name string, v cue.Value, opts ...cue.Option) *File {
	opts = append(defaultOpts, opts...)
	ctx := cuecontext.New()
	file := &File{
		Name: name,
		ctx:  ctx,
		file: &ast.File{
			Decls: v.Syntax(opts...).(*ast.StructLit).Elts,
		},
		v: v,
	}
	file.setComments(v.Doc())
	return file
}

// src must be a string, []byte, or io.Reader.
// if src is nil, the file is read from filepath.
func parse(ctx *cue.Context, filepath string, src any) (*ast.File, error) {
	tree, err := parser.ParseFile(filepath, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func fieldsDelta(schema, data cue.Value, opts []cue.Option, parents ...cue.Selector) ([]cue.Path, error) {
	m := make([]cue.Path, 0)
	iter, err := schema.Fields(opts...)
	if err != nil {
		return nil, err
	}

	for iter.Next() {
		sel := append(parents, iter.Selector())
		// we need the absolute path to the field for lookup
		// but only the relative path to the field for m (missing fields)
		// as it will be appended to a list containing parent selectors
		absPath := cue.MakePath(sel...)
		relPath := cue.MakePath(append([]cue.Selector{}, iter.Selector())...)

		//skip private fields based on the private attribute
		//e.g. @private(true)
		attr := iter.Value().Attribute(privateAttr)
		if err := attr.Err(); err == nil {
			continue
		}

		entry := data.LookupPath(absPath)
		if !entry.Exists() {
			m = append(m, relPath)
		}

		switch iter.Value().Syntax().(type) {
		case *ast.StructLit:
			// recurse into the struct
			// to find missing fields
			x := schema.LookupPath(absPath)
			n, err := fieldsDelta(x, data, opts, sel...)
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

func makeValues(iter *cue.Iterator, paths []string, schemaVersion string, parents ...cue.Selector) ([]ast.Decl, error) {
	result := make([]ast.Decl, 0)
	for iter.Next() {
		var v ast.Expr
		value := iter.Value()
		sel := append(parents, iter.Selector())
		path := cue.MakePath(sel...)

		if !slices.Contains(paths, path.String()) {
			continue
		}

		field, hasDefaultValue := value.Default()

		if !hasDefaultValue && value.IsConcrete() {
			switch value.Syntax(cue.Raw()).(type) {
			case *ast.StructLit:
				var rx []ast.Decl
				f, err := value.Fields(defaultOpts...)
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
