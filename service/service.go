package service

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

// RpcMethod includes method, args type, reply type and call times
type RpcMethod struct {
	Method    reflect.Method
	ArgsType  reflect.Type
	ReplyType reflect.Type
	CallTimes uint64
}

// NewArgsValue is to create args value
func (m *RpcMethod) NewArgsValue() reflect.Value {
	var argsValue reflect.Value
	if m.ArgsType.Kind() == reflect.Ptr {
		argsValue = reflect.New(m.ArgsType.Elem())
	} else {
		argsValue = reflect.New(m.ArgsType).Elem()
	}
	return argsValue
}

// NewReplyValue is to create reply value
func (m *RpcMethod) NewReplyValue() reflect.Value {
	replyValue := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyValue.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyValue.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyValue
}

// GetCallTimes is to get call times
func (m *RpcMethod) GetCallTimes() uint64 {
	return atomic.LoadUint64(&m.CallTimes)
}

// Service includes name, type, instance and rpc methods
type Service struct {
	Name       string
	Type       reflect.Type
	Instance   reflect.Value
	RpcMethods map[string]*RpcMethod
}

// NewService is create service
func NewService(instance interface{}) *Service {
	service := &Service{
		Name:       reflect.ValueOf(instance).Type().Name(),
		Type:       reflect.TypeOf(instance),
		Instance:   reflect.ValueOf(instance),
		RpcMethods: make(map[string]*RpcMethod),
	}

	if !(ast.IsExported(service.Name) || service.Type.PkgPath() == "") {
		log.Fatalf("service: %s is not exported or built in\n", service.Name)
	}

	service.RegisterMethods()

	return service
}

// RegisterMethods is to register methods to service map
func (s *Service) RegisterMethods() {
	for i := 0; i < s.Type.NumMethod(); i++ {
		method := s.Type.Method(i)
		methodType := method.Type
		argsType, replyType := methodType.In(1), methodType.In(2)

		if !s.checkRpcMethodFormat(methodType, argsType, replyType) {
			continue
		}

		s.RpcMethods[method.Name] = &RpcMethod{
			Method:    method,
			ArgsType:  argsType,
			ReplyType: argsType,
			CallTimes: 0,
		}

		log.Printf("service: register %s.%s\n", s.Name, method.Name)
	}
}

func (s *Service) checkRpcMethodFormat(methodType, argsType, replyType reflect.Type) bool {
	if methodType.NumIn() != 3 || methodType.NumOut() != 1 {
		return false
	}

	if !(ast.IsExported(argsType.Name()) || argsType.PkgPath() == "") {
		return false
	}
	if !(ast.IsExported(replyType.Name()) || replyType.PkgPath() == "") {
		return false
	}

	if methodType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		return false
	}

	return true
}

// CallMethod is to call the method from service map
func (s *Service) CallMethod(rpcMethod *RpcMethod, argsValue, replyValue reflect.Value) error {
	atomic.AddUint64(&rpcMethod.CallTimes, 1)

	fn := rpcMethod.Method.Func
	returnValues := fn.Call([]reflect.Value{s.Instance, argsValue, replyValue})
	if errInterface := returnValues[0].Interface(); errInterface != nil {
		return errInterface.(error)
	}
	return nil
}
