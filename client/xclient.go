package client

import (
	"context"
	"gingle-rpc/codec"
	"gingle-rpc/loadbalance"
	"io"
	"reflect"
	"sync"
)

// XClient includes option, load balance, algorithm mode, clients and mutex
type XClient struct {
	opt *codec.Option

	lb   loadbalance.LoadBalance
	mode loadbalance.LbAlgo

	clients map[string]*Client

	mu sync.Mutex
}

var _ io.Closer = (*XClient)(nil)

// NewXClient is to create xclient
func NewXClient(lb loadbalance.LoadBalance, mode loadbalance.LbAlgo, opt *codec.Option) *XClient {
	return &XClient{
		opt:     opt,
		lb:      lb,
		mode:    mode,
		clients: make(map[string]*Client),
	}
}

var _ io.Closer = (*Client)(nil)

// Close is to close xclient's connection
func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()

	for key, client := range xc.clients {
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}

// Dail is to connect client in or not in map
func (xc *XClient) Dial(pattern string) (*Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()

	client, ok := xc.clients[pattern]

	// not works
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(xc.clients, pattern)
		client = nil
	}

	// not found
	if client == nil {
		client, err := XDial(pattern, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[pattern] = client
	}

	return client, nil
}

// PeerToPeer is to call service method for one server
func (xc *XClient) PeerToPeer(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	server, err := xc.lb.GetOne(xc.mode)
	if err != nil {
		return err
	}

	client, err := xc.Dial(server)
	if err != nil {
		return err
	}

	return client.Call(ctx, serviceMethod, args, reply)
}

// Broadcast is to call service method for all servers
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	var globalErr error
	isReplyNil := reply == nil
	ctx, cancel := context.WithCancel(ctx)

	servers, globalErr := xc.lb.GetAll()
	if globalErr != nil {
		return globalErr
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Add(1)
		go func(server string) {
			wg.Done()

			var replyCopy interface{}
			if reply != nil {
				replyCopy = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}

			err := xc.PeerToPeer(ctx, serviceMethod, args, replyCopy)

			mu.Lock()
			if err != nil && globalErr == nil {
				globalErr = err
				cancel()
			}

			if err == nil && !isReplyNil {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(replyCopy).Elem())
				isReplyNil = true
			}
			mu.Unlock()
		}(server)
	}
	wg.Wait()

	return globalErr
}
