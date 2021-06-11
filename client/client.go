package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"gingle-rpc/codec"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultHandlePath = "/gingle/handle"
	defaultDebugPath  = "/gingle/debug"
)

// Call includes service method, sequence number, args, reply, error and done
type Call struct {
	ServiceMethod  string
	SequenceNumber uint64

	Args  interface{}
	Reply interface{}

	Error error
	Done  chan *Call
}

func (c *Call) done() {
	c.Done <- c
}

// Client includes sequence number, codec params, mutexs and states
type Client struct {
	seq uint64

	cc     codec.Codec
	opt    *codec.Option
	header codec.Header
	body   codec.Body

	muForCodec sync.Mutex
	muForCall  sync.Mutex

	pending  map[uint64]*Call
	shutdown bool
	closing  bool
}

var _ io.Closer = (*Client)(nil)

// NewClientFunc is to create client with connection and option
type NewClientFunc func(net.Conn, *codec.Option) (*Client, error)

// NewRPCClient is to create rpc client
func NewRPCClient(conn net.Conn, opt *codec.Option) (*Client, error) {
	fn := codec.NewCodecFuncMap[opt.CodecType]
	if fn == nil {
		errMsg := "client: failed to generate codec func"
		log.Printf(errMsg + "\n")
		return nil, fmt.Errorf(errMsg)
	}

	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		errMsg := fmt.Sprintf("client: failed to decode option, err: %v", err)
		log.Printf(errMsg + "\n")
		return nil, fmt.Errorf(errMsg)
	}

	client := &Client{
		seq:     1,
		cc:      fn(conn),
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()

	return client, nil
}

// NewHTTPClient is to create http client
func NewHTTPClient(conn net.Conn, opt *codec.Option) (*Client, error) {
	_, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultHandlePath))

	res, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && res.Status == "200 Connected to Gingle RPC" {
		return NewRPCClient(conn, opt)
	}

	if err == nil {
		err = fmt.Errorf("client: failed to new http client, err: unexpected http response")
	}
	return nil, err
}

// IsAvailable is to check whether client works
func (c *Client) IsAvailable() bool {
	c.muForCall.Lock()
	defer c.muForCall.Unlock()

	return !c.closing && !c.shutdown
}

// Close is to close client's connection
func (c *Client) Close() error {
	c.muForCall.Lock()
	defer c.muForCall.Unlock()

	if !c.IsAvailable() {
		return fmt.Errorf("client: failed to close connection, err: connection has already been closed or shut down")
	}
	c.closing = true
	return c.cc.Close()
}

// Go is to invoke the named function asynchronously
func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call)
	} else if cap(done) == 0 {
		log.Panic("client: done channel is unbuffered")
	}

	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}

	c.send(call)
	return call
}

// Call is to invoke the named function and wait for it to complete
func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	// handle client timeout for call by customed context
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		c.cancelCall(call.SequenceNumber)
		return fmt.Errorf("client: failed to call, err: %v", ctx.Err())
	case call := <-call.Done:
		return call.Error
	}
}

func (c *Client) registerCall(call *Call) (uint64, error) {
	c.muForCall.Lock()
	defer c.muForCall.Unlock()

	if !c.IsAvailable() {
		return 0, fmt.Errorf("client: failed to close connection, err: connection has already been closed or shut down")
	}
	call.SequenceNumber = c.seq
	c.pending[call.SequenceNumber] = call
	c.seq++
	return call.SequenceNumber, nil
}

func (c *Client) cancelCall(seq uint64) *Call {
	c.muForCall.Lock()
	defer c.muForCall.Unlock()

	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

func (c *Client) send(call *Call) {
	c.muForCall.Lock()
	defer c.muForCall.Unlock()

	// register this call
	seq, err := c.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// prepare request header
	c.header.ServiceMethod = call.ServiceMethod
	c.header.SequenceNumber = seq
	c.header.Error = ""

	// prepare request body
	c.body = call.Args

	// encode this request
	if err := c.cc.Write(&c.header, c.body); err != nil {
		call := c.cancelCall(seq)
		if call != nil {
			call.Error = err
			call.done()
			return
		}
	}
}

func (c *Client) receive() {
	var err error
	for err == nil {
		header := &codec.Header{}

		// decode response header
		if err = c.cc.ReadHeader(header); err != nil {
			break
		}

		// cancel this call
		call := c.cancelCall(header.SequenceNumber)

		// decode response body
		switch {
		case call == nil:
			err = c.cc.ReadBody(nil)
		case header.Error != "":
			call.Error = fmt.Errorf(header.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = fmt.Errorf("client: failed to read body, err: %v", err)
			}
			call.done()
		}
	}

	c.muForCodec.Lock()
	defer c.muForCodec.Unlock()
	c.muForCall.Lock()
	defer c.muForCall.Unlock()

	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

type clientChanItem struct {
	client *Client
	err    error
}

func parseOptions(opts ...*codec.Option) (*codec.Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return codec.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, fmt.Errorf("client: parse more one option")
	}

	opt := opts[0]
	opt.MagicNumber = codec.DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = codec.DefaultOption.CodecType
	}
	return opt, nil
}

func dialTimeout(fn NewClientFunc, network, address string, opts ...*codec.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	ch := make(chan clientChanItem)
	go func() {
		client, err = fn(conn, opt)
		ch <- clientChanItem{client: client, err: err}
	}()

	// handle client timeout for connect by defined option
	var item clientChanItem
	if opt.ConnectTimeout == 0 {
		item = <-ch
		return item.client, item.err
	}
	select {
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("client: failed to dail, err: connect timeout expected within %s", opt.ConnectTimeout)
	case item = <-ch:
		return item.client, item.err
	}
}

// DialRPC is to connect the rpc server and parse options
func DialRPC(network, address string, opts ...*codec.Option) (client *Client, err error) {
	return dialTimeout(NewRPCClient, network, address, opts...)
}

// DialHTTP is to connect the http server and parse options
func DialHTTP(network, address string, opts ...*codec.Option) (client *Client, err error) {
	return dialTimeout(NewHTTPClient, network, address, opts...)
}

// Dial is to choose to access http protocol or rpc protocol
func Dial(pattern string, opts ...*codec.Option) (client *Client, err error) {
	pair := strings.Split(pattern, "@")
	if len(pair) != 2 {
		return nil, fmt.Errorf("client: failed to dail, err: dial pattern %s format not correct", pattern)
	}

	protocol, address := pair[0], pair[1]
	switch protocol {
	case "http": // http ->
		return DialHTTP("tcp", address, opts...)
	default: // rpc -> tcp or unix
		return DialRPC(protocol, address, opts...)
	}
}
