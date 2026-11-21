package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlatformTestFailureMessageTruncated(t *testing.T) {
	assert.Equal(t,
		"响应被截断，请关闭模型思考模式或调大输出上限",
		platformTestFailureMessage(errUpstreamTruncated),
	)
}
