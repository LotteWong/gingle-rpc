package codec

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

// JsonCodec includes io closer, bufio writer, json encoder and json decoder
type JsonCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	enc  *json.Encoder
	dec  *json.Decoder
}

var _ Codec = (*JsonCodec)(nil)

// NewJsonCodecFunc is to create json codec with io closer
func NewJsonCodecFunc(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &JsonCodec{
		conn: conn,
		buf:  buf,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}
}

// ReadHeader is to json decode header
func (c *JsonCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

// ReadBody is to json decode body
func (c *JsonCodec) ReadBody(b Body) error {
	return c.dec.Decode(b)
}

// Write is to json encode header and body
func (c *JsonCodec) Write(h *Header, b Body) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()

	if err = c.enc.Encode(h); err != nil {
		log.Printf("json codec: failed to encode header, err: %v\n", err)
		return
	}

	if err = c.enc.Encode(b); err != nil {
		log.Printf("json codec: failed to encode body, err: %v\n", err)
		return
	}

	return
}

// Close is to close io connection
func (c *JsonCodec) Close() error {
	return c.conn.Close()
}
