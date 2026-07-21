package storage

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisClient_URLParsing(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// 1. Standard host:port
	rdb1, err := NewRedisClient(s.Addr(), "", 0)
	assert.NoError(t, err)
	assert.NotNil(t, rdb1)
	_ = rdb1.Close()

	// 2. redis:// URL scheme
	redisURL := "redis://" + s.Addr()
	rdb2, err := NewRedisClient(redisURL, "", 0)
	assert.NoError(t, err)
	assert.NotNil(t, rdb2)
	_ = rdb2.Close()
}
