package eslgo

import (
	"bufio"
	"github.com/pkg/errors"
	"github.com/zenthangplus/eslgo/resource"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"time"
)

const EndOfMessage = "\r\n\r\n"

type TcbsocketConn struct {
	conn   net.Conn
	reader *bufio.Reader
	header *textproto.Reader
}

func NewTcpsocketConn(conn net.Conn) *TcbsocketConn {
	reader := bufio.NewReader(conn)
	header := textproto.NewReader(reader)
	return &TcbsocketConn{
		conn:   conn,
		header: header,
		reader: reader,
	}
}

func (c *TcbsocketConn) ReadResponse() (*resource.RawResponse, error) {
	header, err := c.header.ReadMIMEHeader()
	if err != nil {
		return nil, errors.WithMessage(err, "read mime header error")
	}
	response := &resource.RawResponse{
		Headers: header,
	}

	if contentLength := header.Get("Content-Length"); len(contentLength) > 0 {
		length, err := strconv.Atoi(contentLength)
		if err != nil {
			return response, err
		}
		response.Body = make([]byte, length)
		_, err = io.ReadFull(c.reader, response.Body)
		if err != nil {
			return response, err
		}
	}

	return response, nil
}

func (c *TcbsocketConn) Write(data string) error {
	_, err := c.conn.Write([]byte(data + EndOfMessage))
	return err
}

func (c *TcbsocketConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *TcbsocketConn) Close() error {
	return c.conn.Close()
}

func (c *TcbsocketConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}
