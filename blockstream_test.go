package blockstream

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
)

func TestBlockStream(t *testing.T) {
	const (
		nodesCount  = 5
		blocksCount = 256
		size        = 64
		tkn         = Token("test")
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bs, cids := randBlockstore(t, rand.Reader, blocksCount, size)

	net, err := mocknet.FullMeshConnected(ctx, 5)
	require.Nil(t, err, err)
	hs := net.Hosts()

	nodes := make([]*BlockStream, nodesCount)
	for i, h := range hs {
		nodes[i] = NewBlockStream(h, bs, NewPassingGranter())
	}

	wg := new(sync.WaitGroup)
	once := new(sync.Once)

	ctx, cancel = context.WithCancel(ctx)
	sessions := make([]*Session, nodesCount)
	for i, n := range nodes {
		wg.Add(1)
		go func(i int, n *BlockStream) {
			defer wg.Done()

			peers := make([]peer.ID, 0, nodesCount-1)
			for _, h := range hs {
				if h == n.Host {
					continue
				}
				peers = append(peers, h.ID())
			}

			var er error
			sessions[i], er = n.Session(ctx, peers, tkn)
			if er != nil {
				once.Do(func() {
					err = er
				})
			}
		}(i, n)
	}

	wg.Wait()
	require.Nil(t, err, err)

	chans := make([]<-chan blocks.Block, nodesCount)
	for i, s := range sessions {
		chans[i], err = s.GetBlocks(ctx, cids)
		require.Nil(t, err, err)
	}

	for _, ch := range chans {
		assertChan(t, ch, bs, blocksCount)
	}

	cancel()
	for _, n := range nodes {
		err = n.Close()
		require.Nil(t, err, err)
	}
}