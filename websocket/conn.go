package websocket

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/zenthangplus/eslgo/resource"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"time"
)

type Conn struct {
	conn *websocket.Conn
}

func NewConn(conn *websocket.Conn) *Conn {
	return &Conn{conn: conn}
}

func (c Conn) ReadResponse() (*resource.RawResponse, error) {
	messageType, msg, err := c.conn.ReadMessage()
	if err != nil {
		return nil, errors.WithMessage(err, "read message error")
	}
	if messageType != websocket.TextMessage {
		return nil, fmt.Errorf("message type %d not supported", messageType)
	}
	return c.decodeMsg(msg)
}

func (c Conn) decodeMsg(msg []byte) (*resource.RawResponse, error) {
	reader := bufio.NewReader(bytes.NewReader(msg))
	header, err := textproto.NewReader(reader).ReadMIMEHeader()
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
		_, err = io.ReadFull(reader, response.Body)
		if err != nil {
			return response, err
		}
	}
	return response, nil
}

func (c Conn) Write(data string) error {
	return c.conn.WriteMessage(websocket.TextMessage, []byte(data))
}

func (c Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c Conn) Close() error {
	return c.conn.Close()
}

func (c Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}
