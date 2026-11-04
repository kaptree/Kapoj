package dbtypes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jsonRoundTripConfig struct {
	Threshold int `json:"threshold"`
}

func TestJSONRoundTrip(t *testing.T) {
	in := JSON[jsonRoundTripConfig]{Data: jsonRoundTripConfig{Threshold: 3}}
	v, err := in.Value()
	require.NoError(t, err)
	var out JSON[jsonRoundTripConfig]
	require.NoError(t, out.Scan(v))
	assert.Equal(t, 3, out.Data.Threshold)
}

func TestJSONScanNilIsZeroValue(t *testing.T) {
	j := JSON[jsonRoundTripConfig]{Data: jsonRoundTripConfig{Threshold: 9}}
	require.NoError(t, j.Scan(nil))
	assert.Equal(t, jsonRoundTripConfig{}, j.Data)
}

func TestJSONScanStringAndBytes(t *testing.T) {
	var a, b JSON[jsonRoundTripConfig]
	require.NoError(t, a.Scan([]byte(`{"threshold":1}`)))
	require.NoError(t, b.Scan(`{"threshold":2}`))
	assert.Equal(t, 1, a.Data.Threshold)
	assert.Equal(t, 2, b.Data.Threshold)
}

func TestJSONScanInvalidSource(t *testing.T) {
	var j JSON[jsonRoundTripConfig]
	assert.Error(t, j.Scan([]byte(`{not json`)))
	assert.Error(t, j.Scan(42))
}

func TestJSONGormDataType(t *testing.T) {
	assert.Equal(t, "jsonb", JSON[jsonRoundTripConfig]{}.GormDataType())
}

func TestJSONMarshalsUnderlyingValue(t *testing.T) {
	out, err := json.Marshal(JSON[jsonRoundTripConfig]{Data: jsonRoundTripConfig{Threshold: 3}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"threshold":3}`, string(out))
}
