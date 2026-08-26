// Package graphql — the resolver registry, the query parser, and the
// executor behind the GraphQL endpoint required by AI.md PART 14.
//
// The executor supports the subset of the GraphQL query language this
// API needs and rejects everything else with a clear error rather than
// silently ignoring it: named and anonymous query and mutation
// operations, variable definitions and variable usage, aliases,
// arguments of every literal kind, and nested selection sets used to
// narrow the returned document. Subscriptions, fragments, and
// directives are reported as unsupported.
//
// Resolvers are supplied by the router, one per registered operation,
// so this package holds no business logic and reaches no datastore.

package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Resolver produces the value of one root field. args holds the
// resolved arguments, with variables already substituted.
type Resolver func(ctx context.Context, args map[string]any) (any, error)

// Resolvers maps root field names to their resolver functions. It is
// safe for concurrent registration and lookup.
type Resolvers struct {
	mu     sync.RWMutex
	fields map[string]Resolver
}

// NewResolvers returns an empty resolver set.
func NewResolvers() *Resolvers {
	return &Resolvers{fields: make(map[string]Resolver)}
}

// Register adds a resolver for a root field name. It fails on an empty
// name, a nil function, and a duplicate registration.
func (r *Resolvers) Register(name string, fn Resolver) error {
	if name == "" {
		return fmt.Errorf("graphql: a resolver needs a field name")
	}
	if fn == nil {
		return fmt.Errorf("graphql: resolver for %q is nil", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.fields[name]; ok {
		return fmt.Errorf("graphql: resolver for %q already registered", name)
	}
	r.fields[name] = fn
	return nil
}

// MustRegister adds a resolver and panics if the registration is
// invalid, which is always a programming error in a route declaration.
func (r *Resolvers) MustRegister(name string, fn Resolver) {
	if err := r.Register(name, fn); err != nil {
		panic(err)
	}
}

// Lookup returns the resolver for a root field name.
func (r *Resolvers) Lookup(name string) (Resolver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.fields[name]
	return fn, ok
}

// Names returns every registered field name, sorted, because Go map
// iteration order is random and callers compare these lists.
func (r *Resolvers) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.fields))
	for name := range r.fields {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Missing returns the schema root fields that have no resolver, sorted.
// The built-in SDL field is answered by the executor itself and is
// never reported.
func (r *Resolvers) Missing(s *Schema) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for _, name := range s.FieldNames() {
		if name == SDLFieldName {
			continue
		}
		if _, ok := r.fields[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Request is the standard GraphQL over HTTP request body.
type Request struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
}

// ResponseError is one entry of the GraphQL errors array. It carries
// only client-safe text: an internal cause belongs in the logs.
type ResponseError struct {
	Message string   `json:"message"`
	Path    []string `json:"path,omitempty"`
}

// Response is the standard GraphQL over HTTP response body.
type Response struct {
	Data   map[string]any  `json:"data,omitempty"`
	Errors []ResponseError `json:"errors,omitempty"`
}

// ValueKind distinguishes the forms an argument value can take.
type ValueKind int

const (
	// ValueLiteral is a scalar or enum value written in the query.
	ValueLiteral ValueKind = iota
	// ValueVariable is a $name reference resolved from the variables.
	ValueVariable
	// ValueList is a bracketed list of values.
	ValueList
	// ValueObject is a braced object of named values.
	ValueObject
)

// Value is one parsed argument value.
type Value struct {
	// Kind says which of the remaining members carries the value.
	Kind ValueKind
	// Literal holds a string, float64, bool, or nil for ValueLiteral.
	Literal any
	// Variable holds the variable name, without the dollar sign.
	Variable string
	// List holds the elements of a ValueList.
	List []Value
	// Object holds the members of a ValueObject.
	Object map[string]Value
}

// Resolve turns a parsed value into a concrete Go value, substituting
// variables from the request.
func (v Value) Resolve(vars map[string]any) (any, error) {
	switch v.Kind {
	case ValueVariable:
		got, ok := vars[v.Variable]
		if !ok {
			return nil, fmt.Errorf("variable $%s was not supplied", v.Variable)
		}
		return got, nil
	case ValueList:
		out := make([]any, 0, len(v.List))
		for _, item := range v.List {
			resolved, err := item.Resolve(vars)
			if err != nil {
				return nil, err
			}
			out = append(out, resolved)
		}
		return out, nil
	case ValueObject:
		keys := make([]string, 0, len(v.Object))
		for k := range v.Object {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(v.Object))
		for _, k := range keys {
			resolved, err := v.Object[k].Resolve(vars)
			if err != nil {
				return nil, err
			}
			out[k] = resolved
		}
		return out, nil
	default:
		return v.Literal, nil
	}
}

// Selection is one requested field inside a selection set.
type Selection struct {
	// Alias is the response key when the query renames the field.
	Alias string
	// Name is the field name.
	Name string
	// Args are the field's arguments.
	Args map[string]Value
	// Selections are the nested fields used to narrow the result.
	Selections []Selection
}

// Key returns the response key for a selection, which is the alias when
// one was given and the field name otherwise.
func (s Selection) Key() string {
	if s.Alias != "" {
		return s.Alias
	}
	return s.Name
}

// VarDef is one variable declared by an operation.
type VarDef struct {
	// Name is the variable name without the dollar sign.
	Name string
	// Type is the declared GraphQL type as written in the query.
	Type string
}

// ParsedOperation is one operation of a parsed query document.
type ParsedOperation struct {
	// Type is OperationQuery or OperationMutation.
	Type string
	// Name is the operation name, empty for an anonymous operation.
	Name string
	// Vars are the declared variables.
	Vars []VarDef
	// Selections are the root fields the operation requests.
	Selections []Selection
}

// ParsedQuery is a parsed GraphQL document.
type ParsedQuery struct {
	// Operations are the document's operations in source order.
	Operations []ParsedOperation
}

// Operation returns the operation to execute for a request. An empty
// name selects the only operation in the document.
func (q *ParsedQuery) Operation(name string) (ParsedOperation, error) {
	if len(q.Operations) == 0 {
		return ParsedOperation{}, fmt.Errorf("the document contains no operation")
	}
	if name == "" {
		if len(q.Operations) > 1 {
			return ParsedOperation{}, fmt.Errorf("the document contains %d operations, so operationName is required", len(q.Operations))
		}
		return q.Operations[0], nil
	}
	for _, op := range q.Operations {
		if op.Name == name {
			return op, nil
		}
	}
	return ParsedOperation{}, fmt.Errorf("no operation named %q in the document", name)
}

// Token kinds produced by the lexer.
const (
	tokenEOF = iota
	tokenName
	tokenPunct
	tokenString
	tokenNumber
)

// token is one lexical unit of a query document.
type token struct {
	kind int
	text string
	num  float64
}

// lex splits a query document into tokens. Commas are insignificant in
// GraphQL and are dropped, as are comments.
func lex(src string) ([]token, error) {
	runes := []rune(src)
	var out []token
	i := 0
	for i < len(runes) {
		c := runes[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ',' || c == '\uFEFF':
			i++
		case c == '#':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case c == '"':
			text, next, err := lexString(runes, i)
			if err != nil {
				return nil, err
			}
			out = append(out, token{kind: tokenString, text: text})
			i = next
		case isNameStart(c):
			start := i
			for i < len(runes) && isNameRune(runes[i]) {
				i++
			}
			out = append(out, token{kind: tokenName, text: string(runes[start:i])})
		case c == '-' || (c >= '0' && c <= '9'):
			start := i
			i++
			for i < len(runes) && isNumberRune(runes[i]) {
				i++
			}
			text := string(runes[start:i])
			n, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number %q in query", text)
			}
			out = append(out, token{kind: tokenNumber, text: text, num: n})
		case c == '.':
			if i+2 < len(runes) && runes[i+1] == '.' && runes[i+2] == '.' {
				return nil, fmt.Errorf("fragment spreads are not supported")
			}
			return nil, fmt.Errorf("unexpected %q in query", string(c))
		case strings.ContainsRune("{}()[]:=$!|&@", c):
			out = append(out, token{kind: tokenPunct, text: string(c)})
			i++
		default:
			return nil, fmt.Errorf("unexpected %q in query", string(c))
		}
	}
	out = append(out, token{kind: tokenEOF})
	return out, nil
}

// lexString reads one string literal, block or single line, starting at
// the opening quote, and returns the decoded text and the next index.
func lexString(runes []rune, start int) (string, int, error) {
	if start+2 < len(runes) && runes[start+1] == '"' && runes[start+2] == '"' {
		i := start + 3
		var b strings.Builder
		for i < len(runes) {
			if runes[i] == '"' && i+2 < len(runes) && runes[i+1] == '"' && runes[i+2] == '"' {
				return strings.TrimSpace(b.String()), i + 3, nil
			}
			b.WriteRune(runes[i])
			i++
		}
		return "", 0, fmt.Errorf("unterminated block string in query")
	}

	i := start + 1
	var b strings.Builder
	for i < len(runes) {
		c := runes[i]
		switch c {
		case '"':
			return b.String(), i + 1, nil
		case '\n':
			return "", 0, fmt.Errorf("unterminated string in query")
		case '\\':
			if i+1 >= len(runes) {
				return "", 0, fmt.Errorf("unterminated escape in query")
			}
			i++
			switch runes[i] {
			case '"':
				b.WriteRune('"')
			case '\\':
				b.WriteRune('\\')
			case '/':
				b.WriteRune('/')
			case 'b':
				b.WriteRune('\b')
			case 'f':
				b.WriteRune('\f')
			case 'n':
				b.WriteRune('\n')
			case 'r':
				b.WriteRune('\r')
			case 't':
				b.WriteRune('\t')
			case 'u':
				if i+4 >= len(runes) {
					return "", 0, fmt.Errorf("truncated unicode escape in query")
				}
				code, err := strconv.ParseUint(string(runes[i+1:i+5]), 16, 32)
				if err != nil {
					return "", 0, fmt.Errorf("invalid unicode escape in query")
				}
				b.WriteRune(rune(code))
				i += 4
			default:
				return "", 0, fmt.Errorf("invalid escape %q in query", string(runes[i]))
			}
			i++
		default:
			b.WriteRune(c)
			i++
		}
	}
	return "", 0, fmt.Errorf("unterminated string in query")
}

// isNameStart reports whether a rune may begin a GraphQL name.
func isNameStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isNameRune reports whether a rune may continue a GraphQL name.
func isNameRune(c rune) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}

// isNumberRune reports whether a rune may continue a number literal.
func isNumberRune(c rune) bool {
	return (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-'
}

// parser walks the token stream produced by lex.
type parser struct {
	toks []token
	pos  int
}

// peek returns the current token without consuming it.
func (p *parser) peek() token {
	return p.toks[p.pos]
}

// next consumes and returns the current token.
func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tokenEOF {
		p.pos++
	}
	return t
}

// acceptPunct consumes the current token when it is the given
// punctuation.
func (p *parser) acceptPunct(text string) bool {
	if p.peek().kind == tokenPunct && p.peek().text == text {
		p.pos++
		return true
	}
	return false
}

// expectPunct consumes the given punctuation or reports an error.
func (p *parser) expectPunct(text string) error {
	if p.acceptPunct(text) {
		return nil
	}
	return fmt.Errorf("expected %q in query, found %q", text, p.peek().text)
}

// expectName consumes a name token or reports an error.
func (p *parser) expectName() (string, error) {
	t := p.peek()
	if t.kind != tokenName {
		return "", fmt.Errorf("expected a name in query, found %q", t.text)
	}
	p.pos++
	return t.text, nil
}

// ParseQuery parses a GraphQL document.
func ParseQuery(src string) (*ParsedQuery, error) {
	if strings.TrimSpace(src) == "" {
		return nil, fmt.Errorf("the query is empty")
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	doc := &ParsedQuery{}
	for p.peek().kind != tokenEOF {
		op, err := p.parseOperation()
		if err != nil {
			return nil, err
		}
		doc.Operations = append(doc.Operations, op)
	}
	if len(doc.Operations) == 0 {
		return nil, fmt.Errorf("the document contains no operation")
	}
	return doc, nil
}

// parseOperation parses one query or mutation operation.
func (p *parser) parseOperation() (ParsedOperation, error) {
	op := ParsedOperation{Type: OperationQuery}
	t := p.peek()
	if t.kind == tokenName {
		switch t.text {
		case OperationQuery, OperationMutation:
			op.Type = t.text
			p.next()
		case "subscription":
			return op, fmt.Errorf("subscriptions are not supported")
		case "fragment":
			return op, fmt.Errorf("fragments are not supported")
		default:
			return op, fmt.Errorf("expected query or mutation, found %q", t.text)
		}
		if p.peek().kind == tokenName {
			op.Name = p.next().text
		}
		if p.peek().kind == tokenPunct && p.peek().text == "(" {
			vars, err := p.parseVarDefs()
			if err != nil {
				return op, err
			}
			op.Vars = vars
		}
	}
	sels, err := p.parseSelectionSet()
	if err != nil {
		return op, err
	}
	op.Selections = sels
	return op, nil
}

// parseVarDefs parses a variable definition list.
func (p *parser) parseVarDefs() ([]VarDef, error) {
	if err := p.expectPunct("("); err != nil {
		return nil, err
	}
	var out []VarDef
	for !p.acceptPunct(")") {
		if p.peek().kind == tokenEOF {
			return nil, fmt.Errorf("unterminated variable definitions in query")
		}
		if err := p.expectPunct("$"); err != nil {
			return nil, err
		}
		name, err := p.expectName()
		if err != nil {
			return nil, err
		}
		if err := p.expectPunct(":"); err != nil {
			return nil, err
		}
		typ, err := p.parseVarType()
		if err != nil {
			return nil, err
		}
		if p.acceptPunct("=") {
			if _, err := p.parseValue(); err != nil {
				return nil, err
			}
		}
		out = append(out, VarDef{Name: name, Type: typ})
	}
	return out, nil
}

// parseVarType parses a variable's declared type, which may be a name,
// a list, and any number of non-null markers.
func (p *parser) parseVarType() (string, error) {
	var b strings.Builder
	if p.acceptPunct("[") {
		inner, err := p.parseVarType()
		if err != nil {
			return "", err
		}
		if err := p.expectPunct("]"); err != nil {
			return "", err
		}
		b.WriteString("[" + inner + "]")
	} else {
		name, err := p.expectName()
		if err != nil {
			return "", err
		}
		b.WriteString(name)
	}
	for p.acceptPunct("!") {
		b.WriteString("!")
	}
	return b.String(), nil
}

// parseSelectionSet parses a braced list of selections.
func (p *parser) parseSelectionSet() ([]Selection, error) {
	if err := p.expectPunct("{"); err != nil {
		return nil, err
	}
	var out []Selection
	for !p.acceptPunct("}") {
		if p.peek().kind == tokenEOF {
			return nil, fmt.Errorf("unterminated selection set in query")
		}
		if p.peek().kind == tokenPunct && p.peek().text == "@" {
			return nil, fmt.Errorf("directives are not supported")
		}
		sel, err := p.parseSelection()
		if err != nil {
			return nil, err
		}
		out = append(out, sel)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("a selection set must request at least one field")
	}
	return out, nil
}

// parseSelection parses one field selection with its optional alias,
// arguments, and nested selection set.
func (p *parser) parseSelection() (Selection, error) {
	var sel Selection
	name, err := p.expectName()
	if err != nil {
		return sel, err
	}
	sel.Name = name
	if p.acceptPunct(":") {
		aliased, err := p.expectName()
		if err != nil {
			return sel, err
		}
		sel.Alias = name
		sel.Name = aliased
	}
	if p.peek().kind == tokenPunct && p.peek().text == "(" {
		args, err := p.parseArgs()
		if err != nil {
			return sel, err
		}
		sel.Args = args
	}
	if p.peek().kind == tokenPunct && p.peek().text == "@" {
		return sel, fmt.Errorf("directives are not supported")
	}
	if p.peek().kind == tokenPunct && p.peek().text == "{" {
		nested, err := p.parseSelectionSet()
		if err != nil {
			return sel, err
		}
		sel.Selections = nested
	}
	return sel, nil
}

// parseArgs parses a parenthesised argument list.
func (p *parser) parseArgs() (map[string]Value, error) {
	if err := p.expectPunct("("); err != nil {
		return nil, err
	}
	out := make(map[string]Value)
	for !p.acceptPunct(")") {
		if p.peek().kind == tokenEOF {
			return nil, fmt.Errorf("unterminated argument list in query")
		}
		name, err := p.expectName()
		if err != nil {
			return nil, err
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("argument %q is given twice", name)
		}
		if err := p.expectPunct(":"); err != nil {
			return nil, err
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out[name] = val
	}
	return out, nil
}

// parseValue parses one argument value.
func (p *parser) parseValue() (Value, error) {
	t := p.peek()
	switch {
	case t.kind == tokenPunct && t.text == "$":
		p.next()
		name, err := p.expectName()
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: ValueVariable, Variable: name}, nil
	case t.kind == tokenString:
		p.next()
		return Value{Kind: ValueLiteral, Literal: t.text}, nil
	case t.kind == tokenNumber:
		p.next()
		return Value{Kind: ValueLiteral, Literal: t.num}, nil
	case t.kind == tokenName:
		p.next()
		switch t.text {
		case "true":
			return Value{Kind: ValueLiteral, Literal: true}, nil
		case "false":
			return Value{Kind: ValueLiteral, Literal: false}, nil
		case "null":
			return Value{Kind: ValueLiteral, Literal: nil}, nil
		default:
			return Value{Kind: ValueLiteral, Literal: t.text}, nil
		}
	case t.kind == tokenPunct && t.text == "[":
		p.next()
		val := Value{Kind: ValueList}
		for !p.acceptPunct("]") {
			if p.peek().kind == tokenEOF {
				return Value{}, fmt.Errorf("unterminated list value in query")
			}
			item, err := p.parseValue()
			if err != nil {
				return Value{}, err
			}
			val.List = append(val.List, item)
		}
		return val, nil
	case t.kind == tokenPunct && t.text == "{":
		p.next()
		val := Value{Kind: ValueObject, Object: make(map[string]Value)}
		for !p.acceptPunct("}") {
			if p.peek().kind == tokenEOF {
				return Value{}, fmt.Errorf("unterminated object value in query")
			}
			name, err := p.expectName()
			if err != nil {
				return Value{}, err
			}
			if err := p.expectPunct(":"); err != nil {
				return Value{}, err
			}
			member, err := p.parseValue()
			if err != nil {
				return Value{}, err
			}
			val.Object[name] = member
		}
		return val, nil
	default:
		return Value{}, fmt.Errorf("expected a value in query, found %q", t.text)
	}
}

// Execute runs a request against the schema and the resolver set.
//
// GraphQL reports field failures in the errors array rather than as a
// transport failure, so a partial result and an error list can be
// returned together. Resolver errors are surfaced by message only: the
// resolver is responsible for keeping internal detail out of the text
// it returns, exactly as the REST error envelope requires.
func Execute(ctx context.Context, s *Schema, res *Resolvers, req Request) Response {
	if s == nil {
		return Response{Errors: []ResponseError{{Message: "the server has no GraphQL schema"}}}
	}
	if res == nil {
		return Response{Errors: []ResponseError{{Message: "the server has no GraphQL resolvers"}}}
	}
	doc, err := ParseQuery(req.Query)
	if err != nil {
		return Response{Errors: []ResponseError{{Message: err.Error()}}}
	}
	op, err := doc.Operation(req.OperationName)
	if err != nil {
		return Response{Errors: []ResponseError{{Message: err.Error()}}}
	}
	if op.Type == OperationMutation && len(s.Mutations) == 0 {
		return Response{Errors: []ResponseError{{Message: "this schema defines no mutations"}}}
	}

	out := Response{Data: make(map[string]any, len(op.Selections))}
	for _, sel := range op.Selections {
		key := sel.Key()
		value, err := resolveSelection(ctx, s, res, op, sel, req.Variables)
		if err != nil {
			out.Errors = append(out.Errors, ResponseError{Message: err.Error(), Path: []string{key}})
			out.Data[key] = nil
			continue
		}
		out.Data[key] = value
	}
	return out
}

// resolveSelection resolves one root field and narrows its result to
// the requested subfields.
func resolveSelection(ctx context.Context, s *Schema, res *Resolvers, op ParsedOperation, sel Selection, vars map[string]any) (any, error) {
	if sel.Name == SDLFieldName {
		if op.Type != OperationQuery {
			return nil, fmt.Errorf("field %q is only available on the query root", SDLFieldName)
		}
		return s.SDL(), nil
	}

	field, ok := s.Lookup(op.Type, sel.Name)
	if !ok {
		return nil, fmt.Errorf("cannot select field %q on the %s root", sel.Name, op.Type)
	}

	args := make(map[string]any, len(sel.Args))
	for name, raw := range sel.Args {
		known := false
		for _, def := range field.Args {
			if def.Name == name {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("unknown argument %q on field %q", name, sel.Name)
		}
		value, err := raw.Resolve(vars)
		if err != nil {
			return nil, err
		}
		args[name] = value
	}
	for _, def := range field.Args {
		if !def.Required {
			continue
		}
		if _, ok := args[def.Name]; !ok {
			return nil, fmt.Errorf("argument %q is required on field %q", def.Name, sel.Name)
		}
	}

	fn, ok := res.Lookup(sel.Name)
	if !ok {
		return nil, fmt.Errorf("field %q has no resolver on this server", sel.Name)
	}
	value, err := fn(ctx, args)
	if err != nil {
		return nil, err
	}
	if len(sel.Selections) == 0 {
		return value, nil
	}
	normalised, err := normalise(value)
	if err != nil {
		return nil, err
	}
	return narrow(normalised, sel.Selections)
}

// normalise converts a resolver's Go value into the plain map, slice,
// and scalar shapes the narrowing step walks, using the same JSON
// encoding the REST surface would produce.
func normalise(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("the resolved value cannot be encoded")
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("the resolved value cannot be decoded")
	}
	return out, nil
}

// narrow keeps only the selected subfields of a resolved value,
// recursing through lists and nested objects.
func narrow(v any, sels []Selection) (any, error) {
	switch typed := v.(type) {
	case nil:
		return nil, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			narrowed, err := narrow(item, sels)
			if err != nil {
				return nil, err
			}
			out = append(out, narrowed)
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(sels))
		for _, sel := range sels {
			member, ok := typed[sel.Name]
			if !ok {
				return nil, fmt.Errorf("field %q is not present on the resolved value", sel.Name)
			}
			if len(sel.Selections) == 0 {
				out[sel.Key()] = member
				continue
			}
			narrowed, err := narrow(member, sel.Selections)
			if err != nil {
				return nil, err
			}
			out[sel.Key()] = narrowed
		}
		return out, nil
	default:
		return nil, fmt.Errorf("a scalar value has no subfields to select")
	}
}
