package sbmigrate

import "strings"

type functionBodyLocation struct {
	declarationStart int
	bodyStart        int
	delimiter        string
}

func functionReferenceSearchText(definition string) string {
	location, ok := locateFunctionBody(definition)
	if !ok {
		return definition
	}
	body, ok := extractFunctionBody(definition, location)
	if !ok {
		return definition
	}
	return definition[:location.declarationStart] + "\n" + body
}

func locateFunctionBody(definition string) (functionBodyLocation, bool) {
	for i := 0; i < len(definition); {
		switch {
		case strings.HasPrefix(definition[i:], "--"):
			i = skipLineComment(definition, i+2)
		case strings.HasPrefix(definition[i:], "/*"):
			i = skipBlockComment(definition, i+2)
		case definition[i] == '\'':
			i = skipSingleQuotedLiteral(definition, i+1)
		case definition[i] == '"':
			i = skipDoubleQuotedIdentifier(definition, i+1)
		case definition[i] == '$':
			if _, end, ok := readDollarQuotedLiteral(definition, i); ok {
				i = end
			} else {
				i++
			}
		default:
			if location, ok := functionBodyLocationAt(definition, i); ok {
				return location, true
			}
			i++
		}
	}
	return functionBodyLocation{}, false
}

func functionBodyLocationAt(definition string, start int) (functionBodyLocation, bool) {
	if start+2 >= len(definition) ||
		!strings.EqualFold(definition[start:start+2], "as") ||
		(start > 0 && isSQLIdentifierByte(definition[start-1])) ||
		!isSQLSpace(definition[start+2]) {
		return functionBodyLocation{}, false
	}

	delimiterStart := start + 2
	for delimiterStart < len(definition) && isSQLSpace(definition[delimiterStart]) {
		delimiterStart++
	}
	if delimiterStart >= len(definition) {
		return functionBodyLocation{}, false
	}
	if definition[delimiterStart] == '\'' {
		return functionBodyLocation{
			declarationStart: start,
			bodyStart:        delimiterStart + 1,
			delimiter:        "'",
		}, true
	}

	delimiter, bodyStart, ok := readDollarQuoteDelimiter(definition, delimiterStart)
	if !ok {
		return functionBodyLocation{}, false
	}
	return functionBodyLocation{
		declarationStart: start,
		bodyStart:        bodyStart,
		delimiter:        delimiter,
	}, true
}

func extractFunctionBody(definition string, location functionBodyLocation) (string, bool) {
	if location.delimiter == "'" {
		return extractSingleQuotedFunctionBody(definition, location.bodyStart)
	}

	bodyEnd := strings.Index(definition[location.bodyStart:], location.delimiter)
	if bodyEnd < 0 {
		return "", false
	}
	return definition[location.bodyStart : location.bodyStart+bodyEnd], true
}

func extractSingleQuotedFunctionBody(definition string, bodyStart int) (string, bool) {
	var body strings.Builder
	for i := bodyStart; i < len(definition); i++ {
		if definition[i] != '\'' {
			body.WriteByte(definition[i])
			continue
		}
		if i+1 < len(definition) && definition[i+1] == '\'' {
			body.WriteByte('\'')
			i++
			continue
		}
		return body.String(), true
	}
	return "", false
}

func readDollarQuoteDelimiter(text string, start int) (string, int, bool) {
	if start >= len(text) || text[start] != '$' {
		return "", start, false
	}
	relativeEnd := strings.IndexByte(text[start+1:], '$')
	if relativeEnd < 0 {
		return "", start, false
	}
	end := start + relativeEnd + 2
	delimiter := text[start:end]
	if !validDollarQuoteDelimiter(delimiter) {
		return "", start, false
	}
	return delimiter, end, true
}

func skipDoubleQuotedIdentifier(text string, start int) int {
	for i := start; i < len(text); i++ {
		if text[i] != '"' {
			continue
		}
		if i+1 < len(text) && text[i+1] == '"' {
			i++
			continue
		}
		return i + 1
	}
	return len(text)
}
