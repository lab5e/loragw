package gw

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestState(t *testing.T) {
	assert := require.New(t)
	const testFile = "teststate.json"

	defer os.Remove(testFile)

	state, err := NewStateFromFile(testFile)
	assert.NoError(err)
	assert.NotNil(state)

	state.GatewayID = "100"
	state.SetMapping("1", "2")

	assert.NoError(state.Save(testFile))

	state, err = NewStateFromFile(testFile)
	assert.NoError(err)
	assert.NotNil(state)

	assert.Equal("100", state.GatewayID)
	assert.Len(state.IDMappings, 1)
	assert.Equal("2", state.GetMapping("1"))
	assert.Equal("1", state.GetReverseMapping("2"))

	state.RemoveMapping("1")
	assert.Len(state.IDMappings, 0)
}
