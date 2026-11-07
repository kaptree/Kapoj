package gamequeue

import "context"

// chain runs a list of handlers sequentially. Nil handlers are skipped and the
// first handler to return an error stops the chain, returning that error.
type chain []Handler

// NewChain returns a Handler that calls each non-nil handler in order,
// stopping at the first error.
func NewChain(handlers ...Handler) Handler {
	c := make(chain, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			c = append(c, h)
		}
	}
	return c
}

func (c chain) HandleGameEvent(ctx context.Context, ev Event) error {
	for _, h := range c {
		if err := h.HandleGameEvent(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}
