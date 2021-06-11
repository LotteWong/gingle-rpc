package server

import (
	"encoding/json"
	"fmt"
	"gingle-rpc/codec"
	"gingle-rpc/service"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

const (
	defaultHandlePath = "/gingle/handle"
	defaultDebugPath  = "/gingle/debug"
)

// Call includes header, service, method, args and reply
type Call struct {
	Header *codec.Header

	Service   *service.Service
	RpcMethod *service.RpcMethod

	Args  reflect.Value
	Reply reflect.Value
}

// Server includes services
type Server struct {
	Services sync.Map
}

// NewServer is to create server
func NewServer() *Server {
	return &Server{}
}

// RegisterService is to register service to server map
func (s *Server) RegisterService(instance interface{}) error {
	service := service.NewService(instance)
	if _, ok := s.Services.LoadOrStore(service.Name, service); ok {
		return fmt.Errorf("server: service %s already defined", service.Name)
	}
	return nil
}

// RetrieveService is to retrieve service from server map
func (s *Server) RetrieveService(serviceMethod string) (svc *service.Service, rpcMethod *service.RpcMethod, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = fmt.Errorf("server: service.method %s format not correct", serviceMethod)
		return
	}

	serviceName := serviceMethod[:dot]
	svcInterface, ok := s.Services.Load(serviceName)
	if !ok {
		err = fmt.Errorf("server: service.method %s service not found", serviceMethod)
		return
	}
	svc = svcInterface.(*service.Service)

	methodName := serviceMethod[dot+1:]
	rpcMethod, ok = svc.RpcMethods[methodName]
	if !ok {
		err = fmt.Errorf("server: service.method %s method not found", serviceMethod)
		return
	}

	return
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

// ServeHTTP is to exchange http to rpc protocol
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "CONNECT" {
		w.Header().Set("Context-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 Must Connect\n")
		return
	}

	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		w.Header().Set("Context-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "500 Hijack Connection Failed\n")
		return
	}

	_, _ = io.WriteString(w, "HTTP/1.0 200 Connected to Gingle RPC\n\n")
	s.ServeConn(conn)
}

// HandleHTTP is to register http handlers
func (s *Server) HandleHTTP() {
	http.Handle(defaultHandlePath, s)
	http.Handle(defaultDebugPath, &DebugServer{Server: s})
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

	s.serveCodec(fc(conn), &opt)
}

func (s *Server) serveCodec(cc codec.Codec, opt *codec.Option) {
	mu := new(sync.Mutex)

	wg := new(sync.WaitGroup)
	for {
		call, err := s.readRequest(cc)
		if err != nil {
			if call == nil {
				break
			}

			call.Header.Error = err.Error()
			s.sendResponse(cc, call.Header, struct{}{}, mu)
			continue
		}

		wg.Add(1)
		go s.serveHandler(cc, opt, call, mu, wg)
	}
	wg.Wait()

	_ = cc.Close()
}

func (s *Server) serveHandler(cc codec.Codec, opt *codec.Option, call *Call, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()

	callMethodChan := make(chan struct{})
	sendResponseChan := make(chan struct{})

	go func() {
		err := call.Service.CallMethod(call.RpcMethod, call.Args, call.Reply)
		callMethodChan <- struct{}{}
		if err != nil {
			call.Header.Error = err.Error()
			s.sendResponse(cc, call.Header, struct{}{}, mu)
			sendResponseChan <- struct{}{}
			return
		}
		s.sendResponse(cc, call.Header, call.Reply, mu)
		sendResponseChan <- struct{}{}
	}()

	// handle server timeout for handle by defined option
	if opt.HandleTimeout == 0 {
		<-callMethodChan
		<-sendResponseChan
		return
	}
	select {
	case <-time.After(opt.HandleTimeout):
		call.Header.Error = fmt.Sprintf("server: failed to handle, err: handle timeout expected within %s", opt.HandleTimeout)
		s.sendResponse(cc, call.Header, struct{}{}, mu)
	case <-callMethodChan:
		<-sendResponseChan
	}
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

func (s *Server) readRequestBody(cc codec.Codec, body interface{}) error {
	if err := cc.ReadBody(body); err != nil {
		log.Printf("server: failed to read request body, err: %v\n", err)
		return err
	}

	return nil
}

func (s *Server) readRequest(cc codec.Codec) (*Call, error) {
	var err error
	call := &Call{}

	header, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	call.Header = header

	call.Service, call.RpcMethod, err = s.RetrieveService(call.Header.ServiceMethod)
	if err != nil {
		return call, err
	}
	call.Args = call.RpcMethod.NewArgsValue()
	call.Reply = call.RpcMethod.NewReplyValue()

	argsInterface := call.Args.Interface()
	if call.Args.Type().Kind() != reflect.Ptr {
		argsInterface = call.Args.Addr().Interface()
	}
	err = s.readRequestBody(cc, argsInterface)
	if err != nil {
		return call, err
	}

	return call, nil
}

func (s *Server) sendResponse(cc codec.Codec, header *codec.Header, body codec.Body, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	if err := cc.Write(header, body); err != nil {
		log.Printf("server: failed to send response, err: %v\n", err)
	}
}
