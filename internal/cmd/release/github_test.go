package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGitHub(t *testing.T) {
	gh := newGitHub("abc123")
	assert.Equal(t, "abc123", gh.token)
	assert.NotNil(t, gh.doRequest)
}
