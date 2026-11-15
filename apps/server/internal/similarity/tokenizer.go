// Package similarity detects similarity between contest code submissions by
// tokenizing source files into a normalized token stream.
package similarity

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// TokenKind classifies a token produced by Tokenize.
type TokenKind string

const (
	TokenIdentifier TokenKind = "identifier"
	TokenKeyword    TokenKind = "keyword"
	TokenNumber     TokenKind = "number"
	TokenString     TokenKind = "string"
	TokenChar       TokenKind = "char"
	TokenPunct      TokenKind = "punct"
	TokenPreproc    TokenKind = "preproc"
	// TokenNewline, TokenIndent and TokenDedent are reserved for
	// indentation-sensitive languages; C-like scanners skip whitespace.
	TokenNewline TokenKind = "newline"
	TokenIndent  TokenKind = "indent"
	TokenDedent  TokenKind = "dedent"
)

// Normalized token texts. Identifiers and literals collapse to these markers so
// renamed variables or different literal values do not affect similarity.
const (
	identifierText = "ID"
	numberText     = "NUM"
	stringText     = "STR"
	charText       = "CHR"
	preprocText    = "PP"
)

// Token is one normalized lexical unit of a source file.
type Token struct {
	Kind TokenKind
	Text string
	// Line is the 1-based physical line where the token starts.
	Line int
}

// Tokenize scans source for lang and returns its normalized token stream.
// Comments and whitespace are dropped, literals collapse to NUM/STR/CHR,
// non-keyword identifiers collapse to ID, and C/C++ preprocessor directives
// collapse to a single PP token. Unknown languages return an error.
func Tokenize(lang submission.Language, source string) ([]Token, error) {
	spec, err := specFor(lang)
	if err != nil {
		return nil, err
	}
	return scan(source, spec), nil
}

// languageSpec is the language-specific half of the scanner configuration.
type languageSpec struct {
	keywords map[string]struct{}
	// preprocessor enables C-style # directives at the start of a line.
	preprocessor bool
}

func specFor(lang submission.Language) (languageSpec, error) {
	switch lang {
	case submission.LanguageC, submission.LanguageCPP:
		return languageSpec{keywords: cLikeKeywords, preprocessor: true}, nil
	case submission.LanguageJava:
		return languageSpec{keywords: javaKeywords}, nil
	case submission.LanguageGo:
		return languageSpec{keywords: goKeywords}, nil
	default:
		return languageSpec{}, fmt.Errorf("similarity: unsupported language %q", lang)
	}
}

var (
	cLikeKeywords = keywordSet(`auto break case char const continue default do double
		else enum extern float for goto if inline int long register return short signed
		sizeof static struct switch typedef union unsigned void volatile while class
		namespace public private protected template typename using new delete nullptr
		true false constexpr`)
	javaKeywords = keywordSet(`abstract assert boolean break byte case catch char class
		const continue default do double else enum extends final finally float for goto
		if implements import instanceof int interface long native new package private
		protected public return short static strictfp super switch synchronized this
		throw throws transient try void volatile while true false null`)
	goKeywords = keywordSet(`break case chan const continue default defer else
		fallthrough for func go goto if import interface map package range return select
		struct switch type var true false nil`)
)

// operators is matched longest-first, so every three-character operator must
// precede the two-character ones.
var operators = []string{
	"<<=", ">>=", "&^=", "...",
	"->", "::", "==", "!=", "<=", ">=", "&&", "||", "++", "--", "+=", "-=", "*=", "/=", "%=",
}

func keywordSet(words string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, word := range strings.Fields(words) {
		set[word] = struct{}{}
	}
	return set
}

func scan(source string, spec languageSpec) []Token {
	src := []rune(source)
	tokens := make([]Token, 0, len(src)/3)
	line := 1
	// lineStart tracks whether only whitespace and comments have been seen since
	// the last newline, which is where a C/C++ preprocessor directive may begin.
	lineStart := true

	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			line++
			lineStart = true
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f':
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			sawNewline := false
			for i < len(src) && !(src[i] == '*' && i+1 < len(src) && src[i+1] == '/') {
				if src[i] == '\n' {
					line++
					sawNewline = true
				}
				i++
			}
			if i < len(src) {
				i += 2
			}
			if sawNewline {
				lineStart = true
			}
		case c == '#' && spec.preprocessor && lineStart:
			for i < len(src) && src[i] != '\n' {
				i++
			}
			tokens = append(tokens, Token{Kind: TokenPreproc, Text: preprocText, Line: line})
			lineStart = false
		case c == '"':
			startLine := line
			i, line = scanQuoted(src, i, line, '"')
			tokens = append(tokens, Token{Kind: TokenString, Text: stringText, Line: startLine})
			lineStart = false
		case c == '\'':
			startLine := line
			i, line = scanQuoted(src, i, line, '\'')
			tokens = append(tokens, Token{Kind: TokenChar, Text: charText, Line: startLine})
			lineStart = false
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(src) && unicode.IsDigit(src[i+1])):
			i = scanNumber(src, i)
			tokens = append(tokens, Token{Kind: TokenNumber, Text: numberText, Line: line})
			lineStart = false
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			word := string(src[start:i])
			kind, text := TokenIdentifier, identifierText
			if _, ok := spec.keywords[word]; ok {
				kind, text = TokenKeyword, word
			}
			tokens = append(tokens, Token{Kind: kind, Text: text, Line: line})
			lineStart = false
		default:
			if op := matchOperator(src, i); op != "" {
				tokens = append(tokens, Token{Kind: TokenPunct, Text: op, Line: line})
				i += len(op)
			} else {
				tokens = append(tokens, Token{Kind: TokenPunct, Text: string(c), Line: line})
				i++
			}
			lineStart = false
		}
	}
	return tokens
}

// scanQuoted consumes a string or character literal starting at its opening
// quote and returns the index just past the closing quote. Escapes are skipped
// and newlines inside the literal still advance the line counter.
func scanQuoted(src []rune, i, line int, quote rune) (int, int) {
	for i++; i < len(src); {
		switch src[i] {
		case '\\':
			if i+1 < len(src) {
				if src[i+1] == '\n' {
					line++
				}
				i += 2
				continue
			}
			i++
		case quote:
			return i + 1, line
		case '\n':
			line++
			i++
		default:
			i++
		}
	}
	return i, line
}

// scanNumber consumes a numeric literal, including hex/decimal digits, suffixes
// and exponents such as 1e-9 or 0x1p-3.
func scanNumber(src []rune, i int) int {
	for i < len(src) {
		c := src[i]
		switch {
		case unicode.IsDigit(c) || unicode.IsLetter(c) || c == '.' || c == '_':
			i++
		case (c == '+' || c == '-') && isExponentMarker(src[i-1]):
			i++
		default:
			return i
		}
	}
	return i
}

func isExponentMarker(c rune) bool {
	return c == 'e' || c == 'E' || c == 'p' || c == 'P'
}

func matchOperator(src []rune, i int) string {
	for _, op := range operators {
		if hasRunePrefix(src, i, op) {
			return op
		}
	}
	return ""
}

func hasRunePrefix(src []rune, i int, prefix string) bool {
	runes := []rune(prefix)
	if i+len(runes) > len(src) {
		return false
	}
	for j, r := range runes {
		if src[i+j] != r {
			return false
		}
	}
	return true
}

func isIdentStart(c rune) bool {
	return c == '_' || unicode.IsLetter(c)
}

func isIdentPart(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}
