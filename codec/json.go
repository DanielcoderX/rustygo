package codec

import (
	"errors"
	"io"
	"strconv"
	"unsafe"

	rg "rustygo"
)

type JSONTokenType int

const (
	JSONTokenNone JSONTokenType = iota
	JSONTokenObjectStart
	JSONTokenObjectEnd
	JSONTokenArrayStart
	JSONTokenArrayEnd
	JSONTokenKey
	JSONTokenString
	JSONTokenNumber
	JSONTokenBool
	JSONTokenNull
)

var (
	ErrMalformedJSON = errors.New("codec/json: malformed JSON payload")
)

// JSONScanner is an arena-backed streaming JSON tokenizer.
type JSONScanner struct {
	data []byte
	pos  int
}

// NewJSONScanner creates a new scanner over data.
func NewJSONScanner(data []byte) *JSONScanner {
	return &JSONScanner{
		data: data,
		pos:  0,
	}
}

func (s *JSONScanner) skipWhitespace() {
	for s.pos < len(s.data) {
		b := s.data[s.pos]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == ',' || b == ':' {
			s.pos++
		} else {
			break
		}
	}
}

// Next extracts the next JSON token, storing string/key content into the arena scope.
func (s *JSONScanner) Next(scope *rg.Scope) (JSONTokenType, string, error) {
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return JSONTokenNone, "", io.EOF
	}

	b := s.data[s.pos]
	switch b {
	case '{':
		s.pos++
		return JSONTokenObjectStart, "{", nil
	case '}':
		s.pos++
		return JSONTokenObjectEnd, "}", nil
	case '[':
		s.pos++
		return JSONTokenArrayStart, "[", nil
	case ']':
		s.pos++
		return JSONTokenArrayEnd, "]", nil
	case '"':
		str, err := s.scanString(scope)
		if err != nil {
			return JSONTokenNone, "", err
		}
		// Peek if next non-whitespace is colon ':'
		peekPos := s.pos
		for peekPos < len(s.data) && (s.data[peekPos] == ' ' || s.data[peekPos] == '\t' || s.data[peekPos] == '\r' || s.data[peekPos] == '\n') {
			peekPos++
		}
		if peekPos < len(s.data) && s.data[peekPos] == ':' {
			s.pos = peekPos + 1
			return JSONTokenKey, str, nil
		}
		return JSONTokenString, str, nil
	case 't', 'f':
		return s.scanBool()
	case 'n':
		return s.scanNull()
	default:
		if b == '-' || (b >= '0' && b <= '9') {
			return s.scanNumber()
		}
		return JSONTokenNone, "", ErrMalformedJSON
	}
}

func (s *JSONScanner) scanString(scope *rg.Scope) (string, error) {
	s.pos++ // skip opening quote
	start := s.pos
	escaped := false

	for s.pos < len(s.data) {
		b := s.data[s.pos]
		if b == '\\' {
			escaped = true
			s.pos += 2
			continue
		}
		if b == '"' {
			raw := s.data[start:s.pos]
			s.pos++ // skip closing quote

			if !escaped {
				// Allocate directly in arena
				if scope != nil {
					buf := rg.AllocSlice[byte](scope, len(raw))
					copy(buf, raw)
					return unsafe.String(&buf[0], len(buf)), nil
				}
				return string(raw), nil
			}

			// Unescape if needed
			unquoted, err := strconv.Unquote(`"` + string(raw) + `"`)
			if err != nil {
				return "", err
			}
			if scope != nil {
				buf := rg.AllocSlice[byte](scope, len(unquoted))
				copy(buf, unquoted)
				return unsafe.String(&buf[0], len(buf)), nil
			}
			return unquoted, nil
		}
		s.pos++
	}
	return "", ErrMalformedJSON
}

func (s *JSONScanner) scanBool() (JSONTokenType, string, error) {
	if s.pos+4 <= len(s.data) && string(s.data[s.pos:s.pos+4]) == "true" {
		s.pos += 4
		return JSONTokenBool, "true", nil
	}
	if s.pos+5 <= len(s.data) && string(s.data[s.pos:s.pos+5]) == "false" {
		s.pos += 5
		return JSONTokenBool, "false", nil
	}
	return JSONTokenNone, "", ErrMalformedJSON
}

func (s *JSONScanner) scanNull() (JSONTokenType, string, error) {
	if s.pos+4 <= len(s.data) && string(s.data[s.pos:s.pos+4]) == "null" {
		s.pos += 4
		return JSONTokenNull, "null", nil
	}
	return JSONTokenNone, "", ErrMalformedJSON
}

func (s *JSONScanner) scanNumber() (JSONTokenType, string, error) {
	start := s.pos
	for s.pos < len(s.data) {
		b := s.data[s.pos]
		if (b >= '0' && b <= '9') || b == '-' || b == '+' || b == '.' || b == 'e' || b == 'E' {
			s.pos++
		} else {
			break
		}
	}
	numStr := string(s.data[start:s.pos])
	return JSONTokenNumber, numStr, nil
}
