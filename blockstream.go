package blockstream

import (
	"context"
	"io"
	"sync"

	"github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
)

var log = logging.Logger("blockstream")

const Protocol protocol.ID = "/blockstream/1.0.0"

type BlockStream struct {
	Host    host.Host
	Granter AccessGranter
	Blocks  blockstore.Blockstore

	sessions struct {
		m  map[Token]*Session
		l  sync.Mutex
		wg sync.WaitGroup
	}

	senders struct {
		m  map[Token]*sender
		l  sync.Mutex
		wg sync.WaitGroup
	}

	autosave bool
}

type Option func(plain *BlockStream)

func WithAutoSave() Option {
	return func(bs *BlockStream) {
		bs.autosave = true
	}
}

func NewBlockStream(host host.Host, blocks blockstore.Blockstore, granter AccessGranter, opts ...Option) *BlockStream {
	bs := &BlockStream{
		Host:    host,
		Granter: granter,
		Blocks:  blocks,
		sessions: struct {
			m  map[Token]*Session
			l  sync.Mutex
			wg sync.WaitGroup
		}{m: make(map[Token]*Session)},
		senders: struct {
			m  map[Token]*sender
			l  sync.Mutex
			wg sync.WaitGroup
		}{m: make(map[Token]*sender)},
	}
	for _, opt := range opts {
		opt(bs)
	}

	host.SetStreamHandler(Protocol, func(stream network.Stream) {
		err := bs.handler(stream)
		if err != nil {
			log.Error(err)
		}
	})
	return bs
}

func (bs *BlockStream) Close() error {
	bs.sessions.wg.Wait()
	bs.senders.wg.Wait()
	return nil
}

func (bs *BlockStream) Session(ctx context.Context, peers []peer.ID, token Token) (ses *Session, err error) {
	streams := make([]io.ReadWriteCloser, len(peers))
	for i, p := range peers {
		streams[i], err = bs.Host.NewStream(ctx, p, Protocol)
		if err != nil {
			return
		}
	}

	ses, err = newSession(ctx, streams, token, func(f func() error) {
		if err := f(); err != nil {
			log.Error(err)
		}
	})
	if err != nil {
		return
	}

	bs.sessions.l.Lock()
	bs.sessions.m[token] = ses
	bs.sessions.wg.Add(1)
	bs.sessions.l.Unlock()

	go func() {
		<-ctx.Done()
		bs.sessions.l.Lock()
		delete(bs.sessions.m, token)
		bs.sessions.wg.Done()
		bs.sessions.l.Unlock()
	}()
	return
}

func (bs *BlockStream) handler(stream network.Stream) error {
	var (
		done chan<- error
		tkn  Token
	)
	s, err := newSender(stream, bs.Blocks, maxMsgSize,
		func(tkn Token) (err error) {
			done, err = bs.Granter.Granted(tkn, stream.Conn().RemotePeer())
			return
		},
		func(f func() error) {
			if err := f(); err != nil {
				log.Error(err)
				done <- err
			}

			bs.senders.l.Lock()
			delete(bs.senders.m, tkn)
			bs.senders.wg.Done()
			bs.senders.l.Unlock()
		},
	)
	if err != nil {
		return err
	}

	bs.senders.l.Lock()
	bs.senders.m[s.t] = s
	bs.senders.wg.Add(1)
	bs.senders.l.Unlock()
	return nil
}

type onToken func(Token) error

type onClose func(func() error)