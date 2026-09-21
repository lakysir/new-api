package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTopupGroupRatioSupportsMultipleUserGroups(t *testing.T) {
	original := TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateTopupGroupRatioByJSONString(original))
	})

	require.NoError(t, UpdateTopupGroupRatioByJSONString(`{
		"default": 1.03,
		"vip": 0.9,
		"default,vip": 0.8
	}`))

	tests := []struct {
		name       string
		userGroups string
		expected   float64
	}{
		{name: "single group", userGroups: "default", expected: 1.03},
		{name: "existing composite configuration", userGroups: "default,vip", expected: 0.8},
		{name: "first configured group", userGroups: "unknown, vip, default", expected: 0.9},
		{name: "fallback for unknown groups", userGroups: "unknown,missing", expected: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetTopupGroupRatio(tt.userGroups))
		})
	}
}
