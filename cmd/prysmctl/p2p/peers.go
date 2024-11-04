package p2p

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
)

func (c *client) connectToPeers(ctx context.Context, rawPeerAddrs ...string) error {
	addrInfos, err := p2p.ParseGenericAddrs(rawPeerAddrs)
	if err != nil {
		return err
	}
	for _, info := range addrInfos {
		if info.ID == c.host.ID() {
			continue
		}
		if err := c.host.Connect(ctx, info); err != nil {
			return err
		}
	}
	return nil
}
