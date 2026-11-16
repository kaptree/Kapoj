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

// Structural token markers for indentation-sensitive languages. Their exact
// text only has to stay stable within one language.
const (
	newlineMarker = "\n"
	indentMarker  = "INDENT"
	dedentMarker  = "DEDENT"
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
// collapse to a single PP token. Python additionally keeps INDENT, DEDENT and
// NEWLINE tokens so block structure is part of the stream. Unknown languages
// return an error.
func Tokenize(lang submission.Language, source string) ([]Token, error) {
	spec, err := specFor(lang)
	if err != nil {
		return nil, err
	}
	if spec.python {
		return scanPython(source), nil
	}
	return scan(source, spec), nil
}

// languageSpec is the language-specific half of the scanner configuration.
type languageSpec struct {
	keywords map[string]struct{}
	// preprocessor enables C-style # directives at the start of a line.
	preprocessor bool
	// python selects the indentation-sensitive scanner and # line comments.
	python bool
}

func specFor(lang submission.Language) (languageSpec, error) {
	switch lang {
	case submission.LanguageC, submission.LanguageCPP:
		return languageSpec{keywords: cLikeKeywords, preprocessor: true}, nil
	case submission.LanguageJava:
		return languageSpec{keywords: javaKeywords}, nil
	case submission.LanguageGo:
		return languageSpec{keywords: goKeywords}, nil
	case submission.LanguagePython3:
		return languageSpec{keywords: pythonKeywords, python: true}, nil
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
	pythonKeywords = keywordSet(`False None True and as assert async await break class
		continue def del elif else except finally for from global if import in is lambda
		nonlocal not or pass raise return try while with yield match case`)
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

// scanPython scans Python source. Indentation is measured per logical line
// (spaces count one, tabs count four) and emitted as INDENT/DEDENT tokens;
// NEWLINE tokens mark the end of a logical line that produced tokens. Blank
// and comment-only lines never change the indentation level. Bracketed
// expressions continue a logical line, matching Python's implicit joining.
func scanPython(source string) []Token {
	src := []rune(source)
	tokens := make([]Token, 0, len(src)/3)
	line := 1
	indents := []int{0}
	atLineStart := true
	lineHasTokens := false
	bracketDepth := 0

	for i := 0; i < len(src); {
		if atLineStart && bracketDepth == 0 {
			width := 0
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				if src[i] == '\t' {
					width += 4
				} else {
					width++
				}
				i++
			}
			// Blank and comment-only lines are dropped without touching the
			// indentation stack.
			if i >= len(src) || src[i] == '\n' || src[i] == '\r' || src[i] == '#' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
				if i < len(src) {
					i++
					line++
				}
				continue
			}
			for indents[len(indents)-1] < width {
				indents = append(indents, width)
				tokens = append(tokens, Token{Kind: TokenIndent, Text: indentMarker, Line: line})
			}
			for indents[len(indents)-1] > width {
				indents = indents[:len(indents)-1]
				tokens = append(tokens, Token{Kind: TokenDedent, Text: dedentMarker, Line: line})
			}
			atLineStart = false
		}

		c := src[i]
		switch {
		case c == '\n':
			if bracketDepth == 0 {
				if lineHasTokens {
					tokens = append(tokens, Token{Kind: TokenNewline, Text: newlineMarker, Line: line})
					lineHasTokens = false
				}
				atLineStart = true
			}
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f':
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			startLine := line
			if hasTripleQuote(src, i, c) {
				i, line = scanTripleQuoted(src, i, line, c)
			} else {
				i, line = scanQuoted(src, i, line, c)
			}
			tokens = append(tokens, Token{Kind: TokenString, Text: stringText, Line: startLine})
			lineHasTokens = true
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(src) && unicode.IsDigit(src[i+1])):
			i = scanNumber(src, i)
			tokens = append(tokens, Token{Kind: TokenNumber, Text: numberText, Line: line})
			lineHasTokens = true
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			word := string(src[start:i])
			kind, text := TokenIdentifier, identifierText
			if _, ok := pythonKeywords[word]; ok {
				kind, text = TokenKeyword, word
			}
			tokens = append(tokens, Token{Kind: kind, Text: text, Line: line})
			lineHasTokens = true
		default:
			tok := Token{Kind: TokenPunct, Text: string(c), Line: line}
			if op := matchOperator(src, i); op != "" {
				tok.Text = op
				i += len(op)
			} else {
				i++
			}
			switch c {
			case '(', '[', '{':
				bracketDepth++
			case ')', ']', '}':
				if bracketDepth > 0 {
					bracketDepth--
				}
			}
			tokens = append(tokens, tok)
			lineHasTokens = true
		}
	}

	// Python ends the final logical line and closes every open block at EOF.
	if lineHasTokens {
		tokens = append(tokens, Token{Kind: TokenNewline, Text: newlineMarker, Line: line})
	}
	for len(indents) > 1 {
		indents = indents[:len(indents)-1]
		tokens = append(tokens, Token{Kind: TokenDedent, Text: dedentMarker, Line: line})
	}
	return tokens
}

// hasTripleQuote reports whether the quote at i opens a triple-quoted string.
func hasTripleQuote(src []rune, i int, quote rune) bool {
	return i+2 < len(src) && src[i+1] == quote && src[i+2] == quote
}

// scanTripleQuoted consumes a triple-quoted literal, which may span lines.
func scanTripleQuoted(src []rune, i, line int, quote rune) (int, int) {
	i += 3
	for i < len(src) {
		switch {
		case src[i] == '\\' && i+1 < len(src):
			if src[i+1] == '\n' {
				line++
			}
			i += 2
		case src[i] == quote && i+2 < len(src) && src[i+1] == quote && src[i+2] == quote:
			return i + 3, line
		default:
			if src[i] == '\n' {
				line++
			}
			i++
		}
	}
	return i, line
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
