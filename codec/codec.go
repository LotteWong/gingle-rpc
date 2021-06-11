package codec

import (
	"io"
	"time"
)

// | Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod: xxx, SequenceNumber: xxx, Error: xxx} | Body interface{} |
// | <-------     固定 JSON 编码     -------> | <-------                 编码方式由 CodecType 来决定                   -------> |

// | Option | Header1 | Body1 | Header2 | Body2 | Header3 | Body3 | ...

// MagicNumber marks it's a gingle-rpc request
const MagicNumber = 0x3bef5c

// Option includes magic number and codec type
type Option struct {
	MagicNumber    int
	CodecType      string
	ConnectTimeout time.Duration
	HandleTimeout  time.Duration
}

var DefaultOption *Option = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      GobType,
	ConnectTimeout: 10 * time.Second,
}

// Header includes service method, sequence number and error
type Header struct {
	ServiceMethod  string
	SequenceNumber uint64
	Error          string
}

// Body includes data
type Body interface{}

// Codec support io read and io write based on different encoding types
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(Body) error
	Write(*Header, Body) error
}

// NewCodecFunc is to create codec with io closer
type NewCodecFunc func(io.ReadWriteCloser) Codec

const (
	GobType  string = "application/gob"
	JsonType string = "application/gob"
)

// NewCodecFuncMap corresponds encoding types and codec funcs
var NewCodecFuncMap map[string]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[string]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodecFunc
	NewCodecFuncMap[JsonType] = NewJsonCodecFunc
}
