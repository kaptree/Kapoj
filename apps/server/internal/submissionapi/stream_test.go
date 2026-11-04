package submissionapi

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
)

type fakeStreamer struct {
	ch  chan submissionevents.Event
	err error
}

func (f *fakeStreamer) Subscribe(context.Context) (<-chan submissionevents.Event, error) {
	return f.ch, f.err
}

func TestStreamHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	streamer := &fakeStreamer{ch: make(chan submissionevents.Event, 1)}
	h := &handler{streamer: streamer}
	r := gin.New()
	r.GET("/stream", h.stream)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	streamer.ch <- submissionevents.Event{SubmissionID: 42}
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(line, "data: "), line)
	assert.Contains(t, line, `"submissionId":42`)

	// The next read must be a blank line terminating the SSE event.
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "\n", line)

	_ = resp.Body.Close()
}
