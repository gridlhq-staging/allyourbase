// Package api implements collection REST query parsing and execution.
package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// parseFilter parses a filter expression string and returns parameterized SQL.
// Example: "status='active' && age>25" → ("status" = $1 AND "age" > $2), ["active", 25]
func parseFilter(tbl *schema.Table, input string) (string, []any, error) {
	node, err := parseFilterNode(tbl, input)
	if err != nil {
		return "", nil, err
	}
	if node == nil {
		return "", nil, nil
	}
	args := make([]any, 0)
	sql := node.toSQLWithArgs(&args)
	return sql, args, nil
}

// Token types
type tokenKind int

const (
	tokIdent  tokenKind = iota // column name
	tokString                  // 'quoted string'
	tokNumber                  // 123, 45.6
	tokBool                    // true, false
	tokNull                    // null
	tokOp                      // =, !=, >, >=, <, <=, ~, !~
	tokAnd                     // &&, AND
	tokOr                      // ||, OR
	tokIn                      // IN
	tokLParen                  // (
	tokRParen                  // )
	tokComma                   // ,
)

type token struct {
	kind  tokenKind
	value string
}

// AST node types.
type filterNode interface {
	toSQL() string
	toSQLWithArgs(args *[]any) string
}

type andNode struct {
	left, right filterNode
}

func (n *andNode) toSQL() string {
	return "(" + n.left.toSQL() + " AND " + n.right.toSQL() + ")"
}

func (n *andNode) toSQLWithArgs(args *[]any) string {
	return "(" + n.left.toSQLWithArgs(args) + " AND " + n.right.toSQLWithArgs(args) + ")"
}

type orNode struct {
	left, right filterNode
}

func (n *orNode) toSQL() string {
	return "(" + n.left.toSQL() + " OR " + n.right.toSQL() + ")"
}

func (n *orNode) toSQLWithArgs(args *[]any) string {
	return "(" + n.left.toSQLWithArgs(args) + " OR " + n.right.toSQLWithArgs(args) + ")"
}

type comparisonNode struct {
	columnName string
	column     string
	op         string
	paramRef   string // e.g., "$1"
	value      any
}

func (n *comparisonNode) toSQL() string {
	return n.column + " " + n.op + " " + n.paramRef
}

func (n *comparisonNode) toSQLWithArgs(args *[]any) string {
	*args = append(*args, n.value)
	return n.column + " " + n.op + " " + fmt.Sprintf("$%d", len(*args))
}

type inNode struct {
	columnName string
	column     string
	paramRefs  []string
	values     []any
}

func (n *inNode) toSQL() string {
	return n.column + " IN (" + strings.Join(n.paramRefs, ", ") + ")"
}

func (n *inNode) toSQLWithArgs(args *[]any) string {
	paramRefs := make([]string, 0, len(n.values))
	for _, value := range n.values {
		*args = append(*args, value)
		paramRefs = append(paramRefs, fmt.Sprintf("$%d", len(*args)))
	}
	return n.column + " IN (" + strings.Join(paramRefs, ", ") + ")"
}

type isNullNode struct {
	columnName string
	column     string
	isNull     bool
}

func (n *isNullNode) toSQL() string {
	if n.isNull {
		return n.column + " IS NULL"
	}
	return n.column + " IS NOT NULL"
}

func (n *isNullNode) toSQLWithArgs(_ *[]any) string {
	return n.toSQL()
}

func buildFilterExcludingFacetColumn(tbl *schema.Table, input, facetColumn string) (string, []any, error) {
	node, err := parseFilterNode(tbl, input)
	if err != nil || node == nil {
		return "", nil, err
	}
	remaining := removeTopLevelFacetEquality(node, facetColumn)
	if remaining == nil {
		return "", nil, nil
	}
	args := make([]any, 0)
	return remaining.toSQLWithArgs(&args), args, nil
}

func parseFilterNode(tbl *schema.Table, input string) (filterNode, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	p := &parser{
		tokens: tokens,
		pos:    0,
		tbl:    tbl,
		args:   make([]any, 0),
	}
	node, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("unexpected token at position %d: %s", p.pos, p.tokens[p.pos].value)
	}
	return node, nil
}

func removeTopLevelFacetEquality(node filterNode, facetColumn string) filterNode {
	terms := flattenTopLevelAnd(node)
	kept := make([]filterNode, 0, len(terms))
	for _, term := range terms {
		if isFacetEqualityOrNullCheck(term, facetColumn) {
			continue
		}
		kept = append(kept, term)
	}
	return joinAndTerms(kept)
}

func flattenTopLevelAnd(node filterNode) []filterNode {
	if and, ok := node.(*andNode); ok {
		left := flattenTopLevelAnd(and.left)
		return append(left, flattenTopLevelAnd(and.right)...)
	}
	return []filterNode{node}
}

func joinAndTerms(terms []filterNode) filterNode {
	if len(terms) == 0 {
		return nil
	}
	node := terms[0]
	for _, term := range terms[1:] {
		node = &andNode{left: node, right: term}
	}
	return node
}

func isFacetEqualityOrNullCheck(node filterNode, facetColumn string) bool {
	switch n := node.(type) {
	case *comparisonNode:
		return n.columnName == facetColumn && n.op == "="
	case *inNode:
		return n.columnName == facetColumn
	case *isNullNode:
		return n.columnName == facetColumn && n.isNull
	default:
		return false
	}
}

const maxFilterDepth = 50 // max nesting depth for parenthesized expressions

// parser is a recursive descent parser for filter expressions.
type parser struct {
	tokens []token
	pos    int
	tbl    *schema.Table
	args   []any
	depth  int
}

func (p *parser) peek() *token {
	if p.pos >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.pos]
}

func (p *parser) advance() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) addArg(val any) string {
	p.args = append(p.args, val)
	return fmt.Sprintf("$%d", len(p.args))
}

// expression = and_expr
func (p *parser) parseExpression() (filterNode, error) {
	return p.parseOrExpr()
}

// or_expr = and_expr (("||" | "OR") and_expr)*
func (p *parser) parseOrExpr() (filterNode, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	for {
		t := p.peek()
		if t == nil || t.kind != tokOr {
			break
		}
		p.advance()
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &orNode{left: left, right: right}
	}

	return left, nil
}

// and_expr = primary (("&&" | "AND") primary)*
func (p *parser) parseAndExpr() (filterNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for {
		t := p.peek()
		if t == nil || t.kind != tokAnd {
			break
		}
		p.advance()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &andNode{left: left, right: right}
	}

	return left, nil
}

// primary = comparison | "(" expression ")"
func (p *parser) parsePrimary() (filterNode, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of filter expression")
	}

	// Parenthesized expression.
	if t.kind == tokLParen {
		p.depth++
		if p.depth > maxFilterDepth {
			return nil, fmt.Errorf("filter expression too deeply nested (max %d levels)", maxFilterDepth)
		}
		p.advance()
		node, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		closing := p.peek()
		if closing == nil || closing.kind != tokRParen {
			return nil, fmt.Errorf("expected closing parenthesis")
		}
		p.advance()
		p.depth--
		return node, nil
	}

	// Must be a comparison: identifier op value
	return p.parseComparison()
}

// comparison = identifier op value | identifier "IN" "(" value ("," value)* ")"
func (p *parser) parseComparison() (filterNode, error) {
	t := p.peek()
	if t == nil || t.kind != tokIdent {
		return nil, fmt.Errorf("expected column name, got %v", t)
	}
	ident := p.advance()

	// Validate column against schema.
	col := p.tbl.ColumnByName(ident.value)
	if col == nil {
		return nil, fmt.Errorf("unknown column: %s", ident.value)
	}
	quotedCol := sqlutil.QuoteIdent(ident.value)

	// Check for IN.
	next := p.peek()
	if next != nil && next.kind == tokIn {
		p.advance() // consume IN

		lp := p.peek()
		if lp == nil || lp.kind != tokLParen {
			return nil, fmt.Errorf("expected '(' after IN")
		}
		p.advance()

		var paramRefs []string
		var values []any
		for {
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			ref := p.addArg(val)
			paramRefs = append(paramRefs, ref)
			values = append(values, val)

			next := p.peek()
			if next == nil {
				return nil, fmt.Errorf("expected ')' to close IN list")
			}
			if next.kind == tokRParen {
				p.advance()
				break
			}
			if next.kind != tokComma {
				return nil, fmt.Errorf("expected ',' or ')' in IN list")
			}
			p.advance()
		}

		return &inNode{columnName: ident.value, column: quotedCol, paramRefs: paramRefs, values: values}, nil
	}

	// Regular comparison operator.
	opTok := p.peek()
	if opTok == nil || opTok.kind != tokOp {
		return nil, fmt.Errorf("expected operator after column %s", ident.value)
	}
	op := p.advance()

	// Parse value.
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	// Handle null comparisons specially.
	if val == nil {
		switch op.value {
		case "=":
			return &isNullNode{columnName: ident.value, column: quotedCol, isNull: true}, nil
		case "!=":
			return &isNullNode{columnName: ident.value, column: quotedCol, isNull: false}, nil
		default:
			return nil, fmt.Errorf("null can only be compared with = or !=")
		}
	}

	// Map ~ and !~ to LIKE/NOT LIKE (PocketBase compatibility).
	sqlOp := op.value
	switch op.value {
	case "~":
		sqlOp = "LIKE"
	case "!~":
		sqlOp = "NOT LIKE"
	}

	ref := p.addArg(val)
	return &comparisonNode{columnName: ident.value, column: quotedCol, op: sqlOp, paramRef: ref, value: val}, nil
}

// parseValue parses a literal value token.
func (p *parser) parseValue() (any, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("expected value, got end of input")
	}

	switch t.kind {
	case tokString:
		p.advance()
		return t.value, nil
	case tokNumber:
		p.advance()
		if strings.Contains(t.value, ".") {
			f, err := strconv.ParseFloat(t.value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", t.value)
			}
			return f, nil
		}
		n, err := strconv.ParseInt(t.value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", t.value)
		}
		return n, nil
	case tokBool:
		p.advance()
		return t.value == "true", nil
	case tokNull:
		p.advance()
		return nil, nil
	default:
		return nil, fmt.Errorf("expected value, got %s", t.value)
	}
}
