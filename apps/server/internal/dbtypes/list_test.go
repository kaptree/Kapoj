package dbtypes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListValueNilIsEmptyArray(t *testing.T) {
	var l List[string]
	v, err := l.Value()
	require.NoError(t, err)
	assert.Equal(t, "[]", string(v.([]byte)))
}

func TestListValueRoundTrip(t *testing.T) {
	v, err := List[string]{"a", "b"}.Value()
	require.NoError(t, err)

	var back List[string]
	require.NoError(t, back.Scan(v))
	assert.Equal(t, List[string]{"a", "b"}, back)
}

func TestListScanBytesAndString(t *testing.T) {
	var a, b List[int]
	require.NoError(t, a.Scan([]byte(`[1,2]`)))
	require.NoError(t, b.Scan(`[3]`))
	assert.Equal(t, List[int]{1, 2}, a)
	assert.Equal(t, List[int]{3}, b)
}

func TestListScanNilIsEmpty(t *testing.T) {
	l := List[string]{"stale"}
	require.NoError(t, l.Scan(nil))
	assert.NotNil(t, l)
	assert.Empty(t, l)
}

func TestListScanInvalidJSON(t *testing.T) {
	var l List[string]
	assert.Error(t, l.Scan([]byte(`{not json`)))
	assert.Error(t, l.Scan(42))
}

func TestListGormDataType(t *testing.T) {
	assert.Equal(t, "jsonb", List[string]{}.GormDataType())
}

func TestListMarshalsAsPlainArray(t *testing.T) {
	out, err := json.Marshal(List[string]{"x"})
	require.NoError(t, err)
	assert.JSONEq(t, `["x"]`, string(out))
}
