package model

import (
	"context"
	"sync"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// Fake returns scripted CompletionResponses in order.
type Fake struct {
	mu    sync.Mutex
	Queue []ports.CompletionResponse
	Calls int
}

func (f *Fake) Complete(_ context.Context, _ ports.CompletionRequest) (ports.CompletionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if len(f.Queue) == 0 {
		return ports.CompletionResponse{Content: "(fake model: empty queue)"}, nil
	}
	r := f.Queue[0]
	f.Queue = f.Queue[1:]
	return r, nil
}
