package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaKindFromContentType(t *testing.T) {
	require.Equal(t, "video", mediaKindFromContentType("video/mp4"))
	require.Equal(t, "image", mediaKindFromContentType("IMAGE/PNG; charset=binary"))
	require.Equal(t, "audio", mediaKindFromContentType("audio/mpeg"))
	require.Empty(t, mediaKindFromContentType("application/octet-stream"))
}
