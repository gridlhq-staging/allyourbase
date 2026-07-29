package sbmigrate

import (
	"regexp"
	"sort"
	"strings"
)

var schemaQualifiedReferencePattern = regexp.MustCompile(
	`(?i)(?:^|[^[:alnum:]_$])"?([[:alpha:]_][[:alnum:]_$]*)"?[[:space:]]*\.`,
)

var relationAliasPattern = regexp.MustCompile(
	`(?is)\b(?:from|join|update|using)\s+(?:only\s+)?` +
		`(?:"[^"]+"|[[:alpha:]_][[:alnum:]_$]*)` +
		`(?:\s*\.\s*(?:"[^"]+"|[[:alpha:]_][[:alnum:]_$]*))?` +
		`\s+(?:as\s+)?"?([[:alpha:]_][[:alnum:]_$]*)"?`,
)

var commonTableExpressionPattern = regexp.MustCompile(
	`(?is)(?:\bwith\s+(?:recursive\s+)?|,)\s*"?([[:alpha:]_][[:alnum:]_$]*)"?` +
		`(?:\s*\([^)]*\))?\s+as\s+(?:(?:not\s+)?materialized\s+)?\(`,
)

var qualifiedRelationPrefixPattern = regexp.MustCompile(
	`(?is)\b(?:from|into|join|table|update|using)\s+(?:only\s+)?"?$`,
)

func excludedSchemaReference(definition string) (string, bool) {
	searchText := stripFunctionReferenceIgnorableText(functionReferenceSearchText(definition))
	references := schemaQualifiedReferencePattern.FindAllStringSubmatchIndex(searchText, -1)
	excluded := make(map[string]struct{})
	for _, reference := range references {
		schema := strings.ToLower(searchText[reference[2]:reference[3]])
		if schemaReferenceIsLocalQualifier(searchText, reference[2], reference[1], schema) {
			continue
		}
		if isExcludedFunctionReferenceSchema(schema) {
			excluded[schema] = struct{}{}
		}
	}

	schemas := make([]string, 0, len(excluded))
	for schema := range excluded {
		schemas = append(schemas, schema)
	}
	sort.Strings(schemas)
	if len(schemas) == 0 {
		return "", false
	}
	return schemas[0], true
}

func schemaReferenceIsLocalQualifier(text string, referenceStart, referenceEnd int, schema string) bool {
	if qualifiedReferenceCallsFunction(text, referenceEnd) {
		return false
	}
	if qualifiedReferenceNamesRelation(text, referenceStart) {
		return false
	}
	statement := containingSQLStatement(text, referenceEnd)
	return patternCapturesIdentifier(relationAliasPattern, statement, schema) ||
		patternCapturesIdentifier(commonTableExpressionPattern, statement, schema)
}

func qualifiedReferenceNamesRelation(text string, referenceStart int) bool {
	if qualifiedRelationPrefixPattern.MatchString(text[:referenceStart]) {
		return true
	}
	statementStart := strings.LastIndex(text[:referenceStart], ";") + 1
	return relationClauseExpectsName(text[statementStart:referenceStart])
}

func relationClauseExpectsName(prefix string) bool {
	depth := 0
	inFromList := false
	expectsRelation := false

	for i := 0; i < len(prefix); {
		switch ch := prefix[i]; {
		case isSQLSpace(ch):
			i++
		case ch == '(':
			depth++
			i++
		case ch == ')':
			if depth > 0 {
				depth--
			}
			i++
		case depth > 0:
			i++
		case ch == ',':
			if inFromList {
				expectsRelation = true
			}
			i++
		default:
			token, next, ok := readSQLWord(prefix, i)
			if !ok {
				i++
				continue
			}
			i = next
			switch token {
			case "from", "using":
				inFromList = true
				expectsRelation = true
			case "join", "into", "update", "table":
				expectsRelation = true
			case "only", "lateral":
				if expectsRelation {
					continue
				}
				inFromList = false
			case "where", "group", "order", "limit", "having", "union", "intersect", "except", "returning", "window", "values", "set":
				inFromList = false
				expectsRelation = false
			default:
				if expectsRelation {
					expectsRelation = false
				}
			}
		}
	}

	return expectsRelation
}

func readSQLWord(text string, start int) (string, int, bool) {
	if start >= len(text) || !isSQLIdentifierByte(text[start]) {
		return "", start, false
	}
	end := start + 1
	for end < len(text) && isSQLIdentifierByte(text[end]) {
		end++
	}
	return strings.ToLower(text[start:end]), end, true
}

func patternCapturesIdentifier(pattern *regexp.Regexp, text, identifier string) bool {
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(match[1], identifier) {
			return true
		}
	}
	return false
}

func qualifiedReferenceCallsFunction(text string, referenceEnd int) bool {
	pos := referenceEnd
	for pos < len(text) && isSQLSpace(text[pos]) {
		pos++
	}
	if pos < len(text) && text[pos] == '"' {
		pos++
		for pos < len(text) && text[pos] != '"' {
			pos++
		}
		if pos < len(text) {
			pos++
		}
	} else {
		for pos < len(text) && isSQLIdentifierByte(text[pos]) {
			pos++
		}
	}
	for pos < len(text) && isSQLSpace(text[pos]) {
		pos++
	}
	return pos < len(text) && text[pos] == '('
}

func containingSQLStatement(text string, pos int) string {
	start := strings.LastIndex(text[:pos], ";") + 1
	end := len(text)
	if relativeEnd := strings.Index(text[pos:], ";"); relativeEnd >= 0 {
		end = pos + relativeEnd
	}
	return text[start:end]
}

func stripFunctionReferenceIgnorableText(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "--"):
			i = skipLineComment(text, i+2)
		case strings.HasPrefix(text[i:], "/*"):
			i = skipBlockComment(text, i+2)
		case text[i] == '\'':
			literal, end := readSingleQuotedLiteral(text, i+1)
			if literalIsPLpgSQLExecuteArgument(text, i) {
				out.WriteString(stripFunctionReferenceIgnorableText(literal))
			}
			i = end
		case text[i] == '"':
			end := skipDoubleQuotedIdentifier(text, i+1)
			out.WriteString(classifiableDoubleQuotedIdentifier(text[i:end]))
			i = end
		case text[i] == '$':
			if literal, end, ok := readDollarQuotedLiteral(text, i); ok {
				if literalIsPLpgSQLExecuteArgument(text, i) {
					out.WriteString(stripFunctionReferenceIgnorableText(literal))
				}
				i = end
			} else {
				out.WriteByte(text[i])
				i++
			}
		default:
			out.WriteByte(text[i])
			i++
		}
	}
	return out.String()
}

func classifiableDoubleQuotedIdentifier(identifier string) string {
	if !simpleDoubleQuotedIdentifier(identifier) {
		return strings.Repeat(" ", len(identifier))
	}
	return identifier
}

func simpleDoubleQuotedIdentifier(identifier string) bool {
	if len(identifier) < 2 || identifier[0] != '"' || identifier[len(identifier)-1] != '"' {
		return false
	}
	for i := 1; i < len(identifier)-1; i++ {
		if !isSQLIdentifierByte(identifier[i]) {
			return false
		}
	}
	return true
}

func literalIsPLpgSQLExecuteArgument(text string, literalStart int) bool {
	return precedingIdentifier(text, literalStart) == "execute"
}

func precedingIdentifier(text string, pos int) string {
	end := pos
	for end > 0 && isSQLSpace(text[end-1]) {
		end--
	}
	start := end
	for start > 0 && isSQLIdentifierByte(text[start-1]) {
		start--
	}
	if start == end {
		return ""
	}
	return strings.ToLower(text[start:end])
}

func isSQLSpace(ch byte) bool {
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' || ch == '\f'
}

func isSQLIdentifierByte(ch byte) bool {
	return ch == '_' ||
		(ch >= '0' && ch <= '9') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= 'a' && ch <= 'z')
}

func skipLineComment(text string, start int) int {
	if newline := strings.IndexByte(text[start:], '\n'); newline >= 0 {
		return start + newline + 1
	}
	return len(text)
}

func skipBlockComment(text string, start int) int {
	depth := 1
	for i := start; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "/*"):
			depth++
			i += len("/*")
		case strings.HasPrefix(text[i:], "*/"):
			depth--
			i += len("*/")
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}
	return len(text)
}

func skipSingleQuotedLiteral(text string, start int) int {
	_, end := readSingleQuotedLiteral(text, start)
	return end
}

func readSingleQuotedLiteral(text string, start int) (string, int) {
	var literal strings.Builder
	for i := start; i < len(text); i++ {
		if text[i] != '\'' {
			literal.WriteByte(text[i])
			continue
		}
		if i+1 < len(text) && text[i+1] == '\'' {
			literal.WriteByte('\'')
			i++
			continue
		}
		return literal.String(), i + 1
	}
	return literal.String(), len(text)
}

func skipDollarQuotedLiteral(text string, start int) (int, bool) {
	_, end, ok := readDollarQuotedLiteral(text, start)
	return end, ok
}

func readDollarQuotedLiteral(text string, start int) (string, int, bool) {
	delimiter, delimiterEnd, ok := readDollarQuoteDelimiter(text, start)
	if !ok {
		return "", start, false
	}
	bodyEnd := strings.Index(text[delimiterEnd:], delimiter)
	if bodyEnd < 0 {
		return text[delimiterEnd:], len(text), true
	}
	body := text[delimiterEnd : delimiterEnd+bodyEnd]
	return body, delimiterEnd + bodyEnd + len(delimiter), true
}

func validDollarQuoteDelimiter(delimiter string) bool {
	if len(delimiter) < 2 || delimiter[0] != '$' || delimiter[len(delimiter)-1] != '$' {
		return false
	}
	for i := 1; i < len(delimiter)-1; i++ {
		ch := delimiter[i]
		if ch != '_' && (ch < '0' || ch > '9') && (ch < 'A' || ch > 'Z') && (ch < 'a' || ch > 'z') {
			return false
		}
	}
	return true
}

func isExcludedFunctionReferenceSchema(schema string) bool {
	if schema == "" || isPostgresCatalogSchema(schema) {
		return false
	}
	return !isAdmittedUserSchema(schema)
}
