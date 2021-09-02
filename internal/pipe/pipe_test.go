package pipe

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChain(t *testing.T) {
	assert.NoError(t, Chain(nil))

	assert.NoError(t, Chain([]Op{
		func() error { return nil },
	}))

	assert.EqualError(t, Chain([]Op{
		func() error { return nil },
		func() error { return errors.New("foo") },
		func() error { return errors.New("bar") },
	}), "foo")
}

func TestErrIf(t *testing.T) {
	assert.NoError(t, ErrIf(true, nil))
	assert.NoError(t, ErrIf(false, errors.New("foo")))
	assert.EqualError(t, ErrIf(true, errors.New("foo")), "foo")
}
