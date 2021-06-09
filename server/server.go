package server

import (
	"encoding/json"
	"fmt"
	"gingle-rpc/codec"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

// Server includes nothing
type Server struct{}

// NewServer is to create server
func NewServer() *Server {
	return &Server{}
}

// Accept is to listen and serve client connection
func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("server: failed to accept listener, err: %v\n", err)
		}
		go s.ServeConn(conn)
	}
}

// ServeConn is to parse option, choose a codec func and serve codec
func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()

	var opt codec.Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Printf("server: failed to decode option, err: %v\n", err)
		return
	}
	if opt.MagicNumber != codec.MagicNumber {
		log.Printf("server: failed to verify magic number, num: %d\n", opt.MagicNumber)
		return
	}

	fc, ok := codec.NewCodecFuncMap[opt.CodecType]
	if fc == nil || !ok {
		log.Printf("server: failed to generate codec func\n")
		return
	}

	s.serveCodec(fc(conn))
}

func (s *Server) serveCodec(cc codec.Codec) {
	mu := new(sync.Mutex)

	wg := new(sync.WaitGroup)
	for {
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}

			req.header.Error = err.Error()
			s.sendResponse(cc, req.header, struct{}{}, mu)
			continue
		}

		wg.Add(1)
		go s.serveHandler(cc, req, mu, wg)
	}
	wg.Wait()

	_ = cc.Close()
}

func (s *Server) serveHandler(cc codec.Codec, d *Dto, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()

	d.reply = reflect.ValueOf(fmt.Sprintf("%d", d.header.SequenceNumber))
	s.sendResponse(cc, d.header, d.reply, mu)
}

type Dto struct {
	header *codec.Header
	args   reflect.Value
	reply  reflect.Value
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	header := &codec.Header{}

	if err := cc.ReadHeader(header); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Printf("server: failed to read request header, err: %v\n", err)
		}
		return nil, err
	}

	return header, nil
}

func (s *Server) readRequestBody(cc codec.Codec) (*reflect.Value, error) {
	body := reflect.New(reflect.TypeOf(""))

	if err := cc.ReadBody(body.Interface()); err != nil {
		log.Printf("server: failed to read request body, err: %v\n", err)
		return nil, err
	}

	return &body, nil
}

func (s *Server) readRequest(cc codec.Codec) (*Dto, error) {
	req := &Dto{}

	header, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req.header = header

	body, err := s.readRequestBody(cc)
	if err != nil {
		return nil, err
	}
	req.args = *body

	return req, nil
}

func (s *Server) sendResponse(cc codec.Codec, header *codec.Header, body codec.Body, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	if err := cc.Write(header, body); err != nil {
		log.Printf("server: failed to send response, err: %v\n", err)
	}
}
