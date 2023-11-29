package eslgo

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"time"
)

type WebsocketConn struct {
	conn *websocket.Conn
}

func NewWebsocketConn(conn *websocket.Conn) *WebsocketConn {
	return &WebsocketConn{conn: conn}
}

func (c WebsocketConn) ReadResponse() (*RawResponse, error) {
	messageType, msg, err := c.conn.ReadMessage()
	if err != nil {
		return nil, errors.WithMessage(err, "read message error")
	}
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return nil, fmt.Errorf("message type %d not supported", messageType)
	}
	return c.decodeMsg(msg)
}

func (c WebsocketConn) decodeMsg(msg []byte) (*RawResponse, error) {
	reader := bufio.NewReader(bytes.NewReader(msg))
	header, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		return nil, errors.WithMessage(err, "read mime header error")
	}

	response := &RawResponse{
		Headers: header,
	}
	if contentLength := header.Get("Content-Length"); len(contentLength) > 0 {
		length, err := strconv.Atoi(contentLength)
		if err != nil {
			return response, errors.WithMessagef(err, "invalid content length in header: %s", contentLength)
		}
		response.Body = make([]byte, length)
		_, err = io.ReadFull(reader, response.Body)
		if err != nil {
			return response, errors.WithMessagef(err, "read msg body by content length failed: %d", length)
		}
	}
	return response, nil
}

func (c WebsocketConn) Write(data string) error {
	return c.conn.WriteMessage(websocket.TextMessage, []byte(data+EndOfMessage))
}

func (c WebsocketConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c WebsocketConn) Close() error {
	return c.conn.Close()
}

func (c WebsocketConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}
