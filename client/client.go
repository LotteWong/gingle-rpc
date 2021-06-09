package client

import (
	"encoding/json"
	"fmt"
	"gingle-rpc/codec"
	"io"
	"log"
	"net"
	"sync"
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

// NewClient is to create client
func NewClient(conn net.Conn, opt *codec.Option) (*Client, error) {
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
func (c *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-c.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
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

// Dial is to connect the server and parse options
func Dial(network, address string, opts ...*codec.Option) (client *Client, err error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}

	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, opt)
}
