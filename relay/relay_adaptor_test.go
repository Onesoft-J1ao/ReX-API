package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/unrealspeech"
	"github.com/stretchr/testify/require"
)

// GetAdaptor had no case for UnrealSpeech, so a channel of that type fell
// through to nil and every sync (non-task) request failed before reaching the
// upstream, even though the adaptor package and the channel→apiType mapping
// both existed.
func TestGetAdaptorReturnsTheUnrealSpeechAdaptor(t *testing.T) {
	adaptor := GetAdaptor(constant.APITypeUnrealSpeech)

	require.NotNil(t, adaptor, "UnrealSpeech must resolve to an adaptor")
	require.IsType(t, &unrealspeech.Adaptor{}, adaptor)
}
