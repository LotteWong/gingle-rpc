package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

// GobCodec includes io closer, bufio writer, gob encoder and gob decoder
type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	enc  *gob.Encoder
	dec  *gob.Decoder
}

var _ Codec = (*GobCodec)(nil)

// NewGobCodecFunc is to create gob codec with io closer
func NewGobCodecFunc(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}
}

// ReadHeader is to gob decode header
func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

// ReadBody is to gob decode body
func (c *GobCodec) ReadBody(b Body) error {
	return c.dec.Decode(b)
}

// Write is to god encode header and body
func (c *GobCodec) Write(h *Header, b Body) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()

	if err = c.enc.Encode(h); err != nil {
		log.Printf("gob codec: failed to encode header, err: %v\n", err)
		return
	}

	if err = c.enc.Encode(b); err != nil {
		log.Printf("gob codec: failed to encode body, err: %v\n", err)
		return
	}

	return
}

// Close is to close io connection
func (c *GobCodec) Close() error {
	return c.conn.Close()
}
