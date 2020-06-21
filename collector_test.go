package blockstream

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bstore, ids := randBlockstore(t, rand.Reader, 16, 256)

	reqs := make(chan *request, 1)
	newCollector(ctx, reqs, bstore, 512, closeLog)

	req := newRequest(ctx, 0, ids)
	reqs <- req

	for i := 0; i < 8; i++ {
		bs, _ := req.Next()
		for _, b := range bs {
			ok, err := bstore.Has(b.Cid())
			require.Nil(t, err, err)
			assert.True(t, ok)
		}
	}

	req = newRequest(ctx, 0, ids)
	reqs <- req

	for i := 0; i < 8; i++ {
		bs, _ := req.Next()
		for _, b := range bs {
			ok, err := bstore.Has(b.Cid())
			require.Nil(t, err, err)
			assert.True(t, ok)
		}
	}
}
