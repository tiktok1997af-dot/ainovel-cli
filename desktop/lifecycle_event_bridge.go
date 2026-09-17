package main

import (
	"context"
	"errors"

	"github.com/voocel/ainovel-cli/internal/appruntime"
	"github.com/voocel/ainovel-cli/internal/desktopui"
)

type preparedEventBridge struct {
	ctx    context.Context
	cancel context.CancelFunc
	sub    appruntime.EventSubscription
	emit   desktopEventEmitter
}

// prepareEventBridge subscribes the candidate runtime without publishing any
// Gateway bridge state. Lifecycle transitions call this while the session
// controller write lock keeps runtime authority fail-closed.
func (g *Gateway) prepareEventBridge(runtime desktopui.RuntimeClient) (*preparedEventBridge, error) {
	if runtime == nil {
		return nil, nil
	}
	g.mu.Lock()
	if g.sub != nil {
		g.mu.Unlock()
		return nil, errors.New("desktop event bridge is already active")
	}
	parent := g.ctx
	emit := g.emit
	g.mu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	if emit == nil {
		emit = func(context.Context, string, appruntime.DesktopEvent) {}
	}
	bridgeCtx, cancel := context.WithCancel(parent)
	sub, err := runtime.Subscribe(bridgeCtx, appruntime.EventCursor{ContractVersion: appruntime.ContractVersion})
	if err != nil {
		cancel()
		return nil, err
	}
	return &preparedEventBridge{
		ctx:    bridgeCtx,
		cancel: cancel,
		sub:    sub,
		emit:   emit,
	}, nil
}

// activatePreparedEventBridge publishes a previously prepared subscription and
// starts pumping only after the session controller has published the matching
// runtime. It is intentionally non-failing: all fallible subscription work is
// completed in prepareEventBridge before runtime activation.
func (g *Gateway) activatePreparedEventBridge(bridge *preparedEventBridge) {
	if bridge == nil {
		return
	}
	done := make(chan struct{})
	g.mu.Lock()
	if g.sub != nil {
		g.mu.Unlock()
		bridge.cancel()
		_ = bridge.sub.Close()
		return
	}
	g.cancel = bridge.cancel
	g.sub = bridge.sub
	g.done = done
	g.mu.Unlock()
	go g.pumpEvents(bridge.ctx, bridge.sub, done, bridge.emit)
}
