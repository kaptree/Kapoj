package submission_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

func TestStatusTerminal(t *testing.T) {
	for _, s := range []submission.Status{
		submission.StatusAccepted,
		submission.StatusWrongAnswer,
		submission.StatusTimeLimitExceeded,
		submission.StatusMemoryLimitExceeded,
		submission.StatusRuntimeError,
		submission.StatusCompileError,
		submission.StatusSystemError,
	} {
		assert.True(t, s.Terminal(), s)
	}
	for _, s := range []submission.Status{submission.StatusPending, submission.StatusJudging} {
		assert.False(t, s.Terminal(), s)
	}
}

func TestLanguageValid(t *testing.T) {
	for _, l := range []submission.Language{
		submission.LanguageC,
		submission.LanguageCPP,
		submission.LanguagePython3,
		submission.LanguageJava,
		submission.LanguageGo,
	} {
		assert.True(t, l.Valid(), l)
	}
	for _, l := range []submission.Language{"", "c++", "python", "rust"} {
		assert.False(t, l.Valid(), l)
	}
}
