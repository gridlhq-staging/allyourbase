package api

import (
	"fmt"
	"strings"
	"unicode"
)

// tokenize breaks the input into tokens.
func tokenize(input string) ([]token, error) {
	var tokens []token
	runes := []rune(input)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		if unicode.IsSpace(ch) {
			i++
			continue
		}
		if next, ok, err := scanStringToken(runes, i, &tokens); err != nil {
			return nil, err
		} else if ok {
			i = next
			continue
		}
		if next, ok := scanPunctuationToken(ch, &tokens); ok {
			i += next
			continue
		}
		if next, ok := scanTwoCharOperatorToken(runes, i, &tokens); ok {
			i = next
			continue
		}
		if next, ok := scanSingleCharOperatorToken(ch, &tokens); ok {
			i += next
			continue
		}
		if next, ok := scanNumberToken(runes, i, ch, &tokens); ok {
			i = next
			continue
		}
		if next, ok := scanWordToken(runes, i, ch, &tokens); ok {
			i = next
			continue
		}

		return nil, fmt.Errorf("unexpected character '%c' at position %d", ch, i)
	}

	return tokens, nil
}

func scanStringToken(runes []rune, start int, tokens *[]token) (int, bool, error) {
	if runes[start] != '\'' {
		return 0, false, nil
	}
	var buf []rune
	j := start + 1
	for j < len(runes) && runes[j] != '\'' {
		if runes[j] == '\\' && j+1 < len(runes) {
			j++
		}
		buf = append(buf, runes[j])
		j++
	}
	if j >= len(runes) {
		return 0, false, fmt.Errorf("unterminated string at position %d", start)
	}
	*tokens = append(*tokens, token{tokString, string(buf)})
	return j + 1, true, nil
}

func scanPunctuationToken(ch rune, tokens *[]token) (int, bool) {
	switch ch {
	case '(':
		*tokens = append(*tokens, token{tokLParen, "("})
		return 1, true
	case ')':
		*tokens = append(*tokens, token{tokRParen, ")"})
		return 1, true
	case ',':
		*tokens = append(*tokens, token{tokComma, ","})
		return 1, true
	default:
		return 0, false
	}
}

func scanTwoCharOperatorToken(runes []rune, start int, tokens *[]token) (int, bool) {
	if start+1 >= len(runes) {
		return 0, false
	}
	switch string(runes[start : start+2]) {
	case "&&":
		*tokens = append(*tokens, token{tokAnd, "&&"})
	case "||":
		*tokens = append(*tokens, token{tokOr, "||"})
	case "!=":
		*tokens = append(*tokens, token{tokOp, "!="})
	case ">=":
		*tokens = append(*tokens, token{tokOp, ">="})
	case "<=":
		*tokens = append(*tokens, token{tokOp, "<="})
	case "!~":
		*tokens = append(*tokens, token{tokOp, "!~"})
	default:
		return 0, false
	}
	return start + 2, true
}

func scanSingleCharOperatorToken(ch rune, tokens *[]token) (int, bool) {
	if ch != '=' && ch != '>' && ch != '<' && ch != '~' {
		return 0, false
	}
	*tokens = append(*tokens, token{tokOp, string(ch)})
	return 1, true
}

func scanNumberToken(runes []rune, start int, ch rune, tokens *[]token) (int, bool) {
	if !unicode.IsDigit(ch) && !(ch == '-' && start+1 < len(runes) && unicode.IsDigit(runes[start+1])) {
		return 0, false
	}
	j := start
	if ch == '-' {
		j++
	}
	for j < len(runes) && (unicode.IsDigit(runes[j]) || runes[j] == '.') {
		j++
	}
	*tokens = append(*tokens, token{tokNumber, string(runes[start:j])})
	return j, true
}

func scanWordToken(runes []rune, start int, ch rune, tokens *[]token) (int, bool) {
	if !unicode.IsLetter(ch) && ch != '_' {
		return 0, false
	}
	j := start
	for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_' || runes[j] == '.') {
		j++
	}
	word := string(runes[start:j])
	switch upper := strings.ToUpper(word); upper {
	case "AND":
		*tokens = append(*tokens, token{tokAnd, "AND"})
	case "OR":
		*tokens = append(*tokens, token{tokOr, "OR"})
	case "IN":
		*tokens = append(*tokens, token{tokIn, "IN"})
	case "TRUE", "FALSE":
		*tokens = append(*tokens, token{tokBool, strings.ToLower(word)})
	case "NULL":
		*tokens = append(*tokens, token{tokNull, "null"})
	default:
		*tokens = append(*tokens, token{tokIdent, word})
	}
	return j, true
}
