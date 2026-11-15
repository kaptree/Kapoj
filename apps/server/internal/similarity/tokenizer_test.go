package similarity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// tokenPair is the asserted shape of a token: line numbers are deliberately
// excluded so renamed or reflowed code still compares equal.
type tokenPair struct {
	Kind TokenKind
	Text string
}

func tokenPairs(tokens []Token) []tokenPair {
	pairs := make([]tokenPair, len(tokens))
	for i, tok := range tokens {
		pairs[i] = tokenPair{Kind: tok.Kind, Text: tok.Text}
	}
	return pairs
}

func tokenTexts(tokens []Token) []string {
	texts := make([]string, len(tokens))
	for i, tok := range tokens {
		texts[i] = tok.Text
	}
	return texts
}

func TestTokenizeCNormalizesRenamedVariables(t *testing.T) {
	a := `#include <bits/stdc++.h>
int a = 1;
// same logic
int main() { int b = a + 2; return b; }`
	b := `#include <cstdio>
int x = 1;
int main() { int y = x + 2; return y; }`
	ta, err := Tokenize(submission.LanguageC, a)
	require.NoError(t, err)
	tb, err := Tokenize(submission.LanguageC, b)
	require.NoError(t, err)
	assert.Equal(t, tokenPairs(ta), tokenPairs(tb))
}

func TestTokenizeRemovesCommentsAndNormalizesLiterals(t *testing.T) {
	src := `int main(){ // comment
        char c = 'x'; const char* s = "hello"; double n = 3.14; return 0;
    }`
	tokens, err := Tokenize(submission.LanguageCPP, src)
	require.NoError(t, err)
	assert.Contains(t, tokenTexts(tokens), "CHR")
	assert.Contains(t, tokenTexts(tokens), "STR")
	assert.Contains(t, tokenTexts(tokens), "NUM")
	assert.NotContains(t, tokenTexts(tokens), "hello")
}

func TestTokenizeCppPreprocessorLinesBecomePP(t *testing.T) {
	tokens, err := Tokenize(submission.LanguageCPP, "#include <iostream>\n#define F(x) x\nint a;")
	require.NoError(t, err)
	assert.Equal(t, "PP", tokens[0].Text)
	assert.Equal(t, "PP", tokens[1].Text)
	assert.Contains(t, tokenTexts(tokens), "int")
}

func TestTokenizeJavaAndGoKeepKeywords(t *testing.T) {
	javaTokens, err := Tokenize(submission.LanguageJava, "class A { public static void main(String[] args) {} }")
	require.NoError(t, err)
	assert.Contains(t, tokenTexts(javaTokens), "class")
	assert.Contains(t, tokenTexts(javaTokens), "public")
	goTokens, err := Tokenize(submission.LanguageGo, "package main\nfunc main() { var x int }")
	require.NoError(t, err)
	assert.Contains(t, tokenTexts(goTokens), "package")
	assert.Contains(t, tokenTexts(goTokens), "func")
}

func TestTokenizeRejectsUnknownLanguage(t *testing.T) {
	_, err := Tokenize(submission.Language("ruby"), "puts 1")
	assert.Error(t, err)
}

func TestTokenizeTracksLineNumbers(t *testing.T) {
	tokens, err := Tokenize(submission.LanguageC, "int a;\n\nint b;")
	require.NoError(t, err)
	require.Len(t, tokens, 6)
	assert.Equal(t, 1, tokens[0].Line)
	assert.Equal(t, 3, tokens[3].Line)
}

func TestTokenizeUsesLongestOperatorMatch(t *testing.T) {
	cases := map[string]string{
		"a >>= b":   ">>=",
		"a <<= b":   "<<=",
		"a &^= b":   "&^=",
		"f(a, ...)": "...",
		"p->x":      "->",
		"A::b":      "::",
	}
	for source, op := range cases {
		tokens, err := Tokenize(submission.LanguageCPP, source)
		require.NoError(t, err)
		assert.Contains(t, tokenTexts(tokens), op, source)
	}
}
