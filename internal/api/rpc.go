package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

// handleRPC handles POST /rpc/{function}, resolving the named function from the schema cache, building a parameterized SQL call, and dispatching to void or read execution paths.
func (h *Handler) handleRPC(w http.ResponseWriter, r *http.Request) {
	if !requireWriteScope(w, r) {
		return
	}
	fn := h.resolveFunction(w, r)
	if fn == nil {
		return
	}
	notify, ok := h.resolveRPCNotifyContract(w, r)
	if !ok {
		return
	}

	args, ok := decodeRPCArgs(w, r)
	if !ok {
		return
	}

	query, queryArgs, err := buildRPCCall(fn, args)
	if err != nil {
		writeErrorWithDoc(w, http.StatusBadRequest, err.Error(), docURL("/guide/database-rpc"))
		return
	}

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if fn.IsVoid {
		h.executeVoidRPC(w, r, fn, q, done, query, queryArgs)
		return
	}

	h.executeReadRPC(w, r, fn, q, done, query, queryArgs, notify)
}

func decodeRPCArgs(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	// Decode JSON body as named arguments (empty body = no args).
	var args map[string]any
	if r.ContentLength <= 0 {
		return args, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, httputil.MaxBodySize)
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return nil, false
	}
	return args, true
}

type rpcNotifyContract struct {
	action  string
	table   string
	target  *schema.Table
	enabled bool
}

// normalizeRPCNotifyContract enables notify publishing only when both headers
// are present and the action is one of the supported realtime verbs.
func normalizeRPCNotifyContract(headers http.Header) rpcNotifyContract {
	table := strings.TrimSpace(headers.Get("X-Notify-Table"))
	action := strings.ToLower(strings.TrimSpace(headers.Get("X-Notify-Action")))
	if table == "" || action == "" {
		return rpcNotifyContract{}
	}
	switch action {
	case "create", "update", "delete":
		return rpcNotifyContract{
			action:  action,
			table:   table,
			enabled: true,
		}
	default:
		return rpcNotifyContract{}
	}
}

func (h *Handler) resolveRPCNotifyContract(w http.ResponseWriter, r *http.Request) (rpcNotifyContract, bool) {
	notify := normalizeRPCNotifyContract(r.Header)
	if !notify.enabled {
		return notify, true
	}

	cache := h.schema.Get()
	target := cache.TableByNameInSchema(tenant.ActiveSchemaFromContext(r.Context()), notify.table)
	if target == nil {
		writeError(w, http.StatusNotFound, "collection not found: "+notify.table)
		return rpcNotifyContract{}, false
	}
	if err := auth.CheckTableScope(auth.ClaimsFromContext(r.Context()), target.Name); err != nil {
		writeErrorWithDoc(w, http.StatusForbidden, "api key does not have access to table: "+target.Name, docURL("/guide/api-reference"))
		return rpcNotifyContract{}, false
	}
	if !requireWritable(w, target) || !requirePK(w, target) {
		return rpcNotifyContract{}, false
	}

	notify.table = target.Name
	notify.target = target
	return notify, true
}

func (h *Handler) executeVoidRPC(w http.ResponseWriter, r *http.Request, fn *schema.Function, q Querier, done func(error) error, query string, queryArgs []any) {
	_, err := q.Exec(r.Context(), query, queryArgs...)
	if err != nil {
		h.writeRPCDatabaseError(w, err, fn.Name, done, "rpc error")
		return
	}
	if !writeRPCDone(w, done) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// executeReadRPC executes a non-void RPC function, scanning the result as a set or single row and optionally publishing realtime notify events for each returned record.
func (h *Handler) executeReadRPC(w http.ResponseWriter, r *http.Request, fn *schema.Function, q Querier, done func(error) error, query string, queryArgs []any, notify rpcNotifyContract) {
	rows, err := q.Query(r.Context(), query, queryArgs...)
	if err != nil {
		h.writeRPCDatabaseError(w, err, fn.Name, done, "rpc error")
		return
	}

	if fn.ReturnsSet {
		items, err := scanRows(rows)
		rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
		if err != nil {
			h.writeRPCDatabaseError(w, err, fn.Name, done, "rpc scan error")
			return
		}
		if !writeRPCDone(w, done) {
			return
		}
		for _, record := range items {
			h.publishRPCNotifyRecord(r.Context(), notify, record)
		}
		writeJSON(w, http.StatusOK, items)
		return
	}

	// Scalar or single-row return.
	record, err := scanRow(rows)
	rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
	if err != nil {
		h.writeRPCDatabaseError(w, err, fn.Name, done, "rpc scan error")
		return
	}
	if !writeRPCDone(w, done) {
		return
	}
	h.publishRPCNotifyRecord(r.Context(), notify, record)

	writeRPCRecord(w, record)
}

func (h *Handler) publishRPCNotifyRecord(ctx context.Context, notify rpcNotifyContract, record map[string]any) {
	if !notify.enabled || !schema.RecordHasPrimaryKeyValues(notify.target, record) {
		return
	}
	h.publishEvent(ctx, notify.action, notify.table, record, nil)
}

// writeRPCDatabaseError keeps scan-time and query-time PostgreSQL failures on
// the same mapping path so the HTTP contract depends on SQLSTATE, not timing.
func (h *Handler) writeRPCDatabaseError(w http.ResponseWriter, err error, function string, done func(error) error, logMessage string) {
	done(err)
	if mapPGError(w, err) {
		return
	}
	h.logger.Error(logMessage, "error", err, "function", function)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeRPCDone(w http.ResponseWriter, done func(error) error) bool {
	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	return true
}

func writeRPCRecord(w http.ResponseWriter, record map[string]any) {
	if record == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	// If the result has a single column named after the function, unwrap it.
	if len(record) == 1 {
		for _, v := range record {
			writeJSON(w, http.StatusOK, v)
			return
		}
	}
	writeJSON(w, http.StatusOK, record)
}

// resolveFunction looks up the function in the schema cache and validates it exists.
func (h *Handler) resolveFunction(w http.ResponseWriter, r *http.Request) *schema.Function {
	sc := h.schema.Get()
	if sc == nil {
		writeError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return nil
	}

	funcName := chi.URLParam(r, "function")
	fn := sc.FunctionByNameInSchema(tenant.ActiveSchemaFromContext(r.Context()), funcName)
	if fn == nil {
		writeError(w, http.StatusNotFound, "function not found: "+funcName)
		return nil
	}
	return fn
}

// buildRPCCall generates the SQL and args for calling a function.
// For set-returning or OUT-param functions: SELECT * FROM schema.func($1, $2, ...)
// For scalar/void functions: SELECT schema.func($1, $2, ...)
func buildRPCCall(fn *schema.Function, args map[string]any) (string, []any, error) {
	var queryArgs []any
	placeholders := make([]string, len(fn.Parameters))

	for i, param := range fn.Parameters {
		castType, err := normalizeRPCParamType(param.Type)
		if err != nil {
			return "", nil, fmt.Errorf("function %q parameter %q has unsupported type %q", fn.Name, param.Name, param.Type)
		}
		val, ok := args[param.Name]
		if !ok {
			// If param has no name, try positional matching is not supported —
			// require named args for safety.
			if param.Name == "" {
				return "", nil, fmt.Errorf("function %q has unnamed parameters; cannot match by name", fn.Name)
			}
			// Missing param — pass NULL.
			val = nil
		}
		queryArgs = append(queryArgs, coerceRPCArg(val, param.Type))
		// Use explicit cast so pgx text-encodes the value and Postgres handles conversion.
		// VARIADIC params need the VARIADIC keyword so Postgres spreads the array.
		if param.IsVariadic {
			placeholders[i] = fmt.Sprintf("VARIADIC $%d::%s", i+1, castType)
		} else {
			placeholders[i] = fmt.Sprintf("$%d::%s", i+1, castType)
		}
	}

	funcRef := sqlutil.QuoteQualifiedName(fn.Schema, fn.Name)
	argList := strings.Join(placeholders, ", ")

	var query string
	// Use SELECT * FROM for set-returning functions, functions with OUT params,
	// and record-returning functions so columns are unpacked into named fields.
	if fn.ReturnsSet || fn.HasOutParams || fn.ReturnType == "record" {
		query = fmt.Sprintf("SELECT * FROM %s(%s)", funcRef, argList)
	} else {
		query = fmt.Sprintf("SELECT %s(%s)", funcRef, argList)
	}

	return query, queryArgs, nil
}

func normalizeRPCParamType(pgType string) (string, error) {
	pgType = strings.TrimSpace(pgType)
	if pgType == "" {
		return "", fmt.Errorf("empty type")
	}

	arraySuffix := ""
	for strings.HasSuffix(pgType, "[]") {
		arraySuffix += "[]"
		pgType = strings.TrimSpace(strings.TrimSuffix(pgType, "[]"))
	}

	tokens, err := tokenizeRPCType(pgType)
	if err != nil {
		return "", err
	}
	if len(tokens) == 0 {
		return "", fmt.Errorf("empty type")
	}
	if err := validateRPCTypeTokens(tokens); err != nil {
		return "", err
	}

	var b strings.Builder
	for i, token := range tokens {
		switch token.kind {
		case rpcTypeTokenDot:
			b.WriteByte('.')
		case rpcTypeTokenModifier:
			b.WriteString(token.value)
		default:
			if i > 0 && tokens[i-1].kind != rpcTypeTokenDot {
				b.WriteByte(' ')
			}
			b.WriteString(token.value)
		}
	}
	b.WriteString(arraySuffix)
	return b.String(), nil
}

type rpcTypeToken struct {
	kind  int
	value string
}

const (
	rpcTypeTokenIdent = iota
	rpcTypeTokenDot
	rpcTypeTokenModifier
)

func tokenizeRPCType(pgType string) ([]rpcTypeToken, error) {
	tokens := make([]rpcTypeToken, 0, 4)
	for i := 0; i < len(pgType); {
		switch ch := pgType[i]; {
		case ch == ' ':
			i++
		case ch == '.':
			tokens = append(tokens, rpcTypeToken{kind: rpcTypeTokenDot, value: "."})
			i++
		case ch == '"':
			start := i
			i++
			for i < len(pgType) {
				if pgType[i] != '"' {
					i++
					continue
				}
				if i+1 < len(pgType) && pgType[i+1] == '"' {
					i += 2
					continue
				}
				i++
				tokens = append(tokens, rpcTypeToken{kind: rpcTypeTokenIdent, value: pgType[start:i]})
				break
			}
			if i > len(pgType) || pgType[i-1] != '"' {
				return nil, fmt.Errorf("unterminated quoted identifier")
			}
		case ch == '(':
			start := i
			i++
			for i < len(pgType) && pgType[i] != ')' {
				if (pgType[i] < '0' || pgType[i] > '9') && pgType[i] != ' ' && pgType[i] != ',' {
					return nil, fmt.Errorf("invalid modifier")
				}
				i++
			}
			if i >= len(pgType) || pgType[i] != ')' {
				return nil, fmt.Errorf("unterminated modifier")
			}
			i++
			tokens = append(tokens, rpcTypeToken{kind: rpcTypeTokenModifier, value: pgType[start:i]})
		case isRPCTypeBareIdentStart(ch):
			start := i
			i++
			for i < len(pgType) && isRPCTypeBareIdentPart(pgType[i]) {
				i++
			}
			tokens = append(tokens, rpcTypeToken{kind: rpcTypeTokenIdent, value: pgType[start:i]})
		default:
			return nil, fmt.Errorf("invalid character %q", ch)
		}
	}
	return tokens, nil
}

func validateRPCTypeTokens(tokens []rpcTypeToken) error {
	allowedKeywords := map[string]bool{
		"bit": true, "character": true, "day": true, "double": true, "hour": true,
		"interval": true, "minute": true, "month": true, "precision": true,
		"second": true, "time": true, "timestamp": true, "to": true,
		"varying": true, "with": true, "without": true, "year": true, "zone": true,
	}

	expectIdent := true
	allowExtraWords := false
	seenModifier := false
	for i, token := range tokens {
		switch token.kind {
		case rpcTypeTokenDot:
			if expectIdent || i == len(tokens)-1 || tokens[i-1].kind != rpcTypeTokenIdent || tokens[i+1].kind != rpcTypeTokenIdent {
				return fmt.Errorf("invalid qualified type")
			}
			expectIdent = true
		case rpcTypeTokenModifier:
			if expectIdent || seenModifier {
				return fmt.Errorf("invalid type modifier placement")
			}
			seenModifier = true
			allowExtraWords = true
		case rpcTypeTokenIdent:
			if expectIdent {
				expectIdent = false
				continue
			}
			if !allowExtraWords || token.value != strings.ToLower(token.value) || !allowedKeywords[token.value] {
				return fmt.Errorf("invalid trailing type token %q", token.value)
			}
			allowExtraWords = true
		}
		if token.kind == rpcTypeTokenIdent && i == 0 {
			allowExtraWords = true
		}
	}
	if expectIdent {
		return fmt.Errorf("incomplete type")
	}
	return nil
}

func isRPCTypeBareIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isRPCTypeBareIdentPart(ch byte) bool {
	return isRPCTypeBareIdentStart(ch) || ch == '$' || (ch >= '0' && ch <= '9')
}

// coerceRPCArg converts a JSON-decoded value to a Go type that pgx can encode
// for the given PostgreSQL type. JSON decodes numbers as float64 and arrays as
// []any, which pgx cannot always map to PG types without explicit conversion.
func coerceRPCArg(val any, pgType string) any {
	if val == nil {
		return nil
	}

	// Handle array types: convert []any to a typed slice that pgx can encode.
	if strings.HasSuffix(pgType, "[]") {
		arr, ok := val.([]any)
		if !ok {
			return val
		}
		elemType := strings.TrimSuffix(pgType, "[]")
		return coerceArray(arr, elemType)
	}

	// Handle scalar numeric types: JSON float64 -> appropriate Go type.
	if f, ok := val.(float64); ok {
		return coerceNumber(f, pgType)
	}

	return val
}

// coerceArray converts a []any slice to a typed slice based on the element type.
func coerceArray(arr []any, elemType string) any {
	switch elemType {
	case "integer", "int4", "smallint", "int2":
		out := make([]int32, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = int32(f)
			}
		}
		return out
	case "bigint", "int8":
		out := make([]int64, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = int64(f)
			}
		}
		return out
	case "real", "float4":
		out := make([]float32, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = float32(f)
			}
		}
		return out
	case "double precision", "float8":
		out := make([]float64, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = f
			}
		}
		return out
	case "text", "varchar", "character varying", "name":
		out := make([]string, len(arr))
		for i, v := range arr {
			if s, ok := v.(string); ok {
				out[i] = s
			}
		}
		return out
	case "boolean", "bool":
		out := make([]bool, len(arr))
		for i, v := range arr {
			if b, ok := v.(bool); ok {
				out[i] = b
			}
		}
		return out
	default:
		// For unrecognized types, convert to string slice and let the cast handle it.
		out := make([]string, len(arr))
		for i, v := range arr {
			out[i] = fmt.Sprint(v)
		}
		return out
	}
}

// coerceNumber converts a JSON float64 to the appropriate Go type for the given PG type.
func coerceNumber(f float64, pgType string) any {
	switch pgType {
	case "integer", "int4", "smallint", "int2", "bigint", "int8":
		if f == math.Trunc(f) {
			return int64(f)
		}
	}
	return f
}
