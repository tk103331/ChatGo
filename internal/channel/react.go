package channel

import "context"

// reactQuietly applies a reaction when the driver supports one and ignores the
// failure otherwise: acknowledging a message is a courtesy, and a platform
// rejecting the emoji must never keep the reply from being sent.
func reactQuietly(ctx context.Context, d Driver, in Inbound, r Reaction) {
	reactor, ok := d.(Reactor)
	if !ok {
		return
	}
	_ = reactor.React(ctx, in, r)
}
