package eslgo

import (
	"bufio"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zenthangplus/eslgo/command"
	"net"
	"strings"
	"testing"
	"time"
)

func createTestResponseHandlerForInbound(conn net.Conn, actualClientRequest chan string) {
	logger := NormalLogger{}
	reader := bufio.NewReader(conn)
	for {
		rawCmd, err := reader.ReadString('\r')
		if err != nil {
			logger.Error("Error when read client request: %s", err)
			break
		}
		cmd := strings.TrimSpace(rawCmd)
		if len(cmd) > 0 {
			actualClientRequest <- cmd
		}
	}
}

func createTestTcpServerForInbound(t *testing.T) (listener net.Listener, connectionCh chan net.Conn) {
	serverAddr := ":0"
	listener, err := net.Listen("tcp", serverAddr)
	if err != nil {
		require.NoError(t, err, "Cannot create listener for tcp server")
	}
	logger := NormalLogger{}
	connectionCh = make(chan net.Conn)
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				break
			}
			logger.Info("Accept incoming connection from: %s", c.LocalAddr())
			connectionCh <- c
		}
		logger.Info("tcp server shutting down")
	}()
	return listener, connectionCh
}

func TestInboundTcp_WhenClientAuthenButServerNotReplyAuthStatus_ShouldCloseConnection(t *testing.T) {
	listener, connectionCh := createTestTcpServerForInbound(t)
	defer listener.Close()

	var clientConn net.Conn
	var actualClientRequestCh = make(chan string)
	go func() {
		select {
		case <-time.After(5 * time.Second):
			require.FailNow(t, "No incoming connection found")
			break
		case clientConn = <-connectionCh:
			go createTestResponseHandlerForInbound(clientConn, actualClientRequestCh)

			_, err := clientConn.Write([]byte("Content-Type: auth/request\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth/request to client")

			authReq := <-actualClientRequestCh
			assert.Equal(t, "auth ClueCon", authReq)

			exitReq := <-actualClientRequestCh
			assert.Equal(t, "exit", exitReq)

			_, err = clientConn.Write([]byte("Content-Type: command/reply\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write exit reply to client")
		}
	}()
	opts := InboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 2 * time.Second,
			Protocol:    Tcpsocket,
		},
		Network:     "tcp",
		Password:    "ClueCon",
		AuthTimeout: 2 * time.Second,
	}
	_, err := opts.Dial(listener.Addr().String())
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestInboundTcp_WhenClientAuthenButServerReplyAuthenFailed_ShouldCloseConnection(t *testing.T) {
	listener, connectionCh := createTestTcpServerForInbound(t)
	defer listener.Close()

	var clientConn net.Conn
	var actualClientRequestCh = make(chan string)
	go func() {
		select {
		case <-time.After(5 * time.Second):
			require.FailNow(t, "No incoming connection found")
			break
		case clientConn = <-connectionCh:
			go createTestResponseHandlerForInbound(clientConn, actualClientRequestCh)

			_, err := clientConn.Write([]byte("Content-Type: auth/request\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth/request to client")

			authReq := <-actualClientRequestCh
			assert.Equal(t, "auth ClueCon", authReq)

			_, err = clientConn.Write([]byte("Content-Type: command/reply\nReply-Text: -ERR invalid\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth failed to client")

			exitReq := <-actualClientRequestCh
			assert.Equal(t, "exit", exitReq)

			_, err = clientConn.Write([]byte("Content-Type: command/reply\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write exit reply to client")
		}
	}()
	opts := InboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 2 * time.Second,
			Protocol:    Tcpsocket,
		},
		Network:     "tcp",
		Password:    "ClueCon",
		AuthTimeout: 2 * time.Second,
	}
	_, err := opts.Dial(listener.Addr().String())
	require.Equal(t, 0, strings.Index(err.Error(), "failed to auth"), "Error should start with 'failed to auth'")
}

func TestInboundTcp_WhenClientAuthenButServerReplyAuthenOk_ShouldEstablishedConnection(t *testing.T) {
	listener, connectionCh := createTestTcpServerForInbound(t)
	defer listener.Close()

	var clientConn net.Conn
	var actualClientRequestCh = make(chan string)
	go func() {
		select {
		case <-time.After(5 * time.Second):
			require.FailNow(t, "No incoming connection found")
			break
		case clientConn = <-connectionCh:
			go createTestResponseHandlerForInbound(clientConn, actualClientRequestCh)

			_, err := clientConn.Write([]byte("Content-Type: auth/request\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth/request to client")

			authReq := <-actualClientRequestCh
			assert.Equal(t, "auth ClueCon", authReq)

			_, err = clientConn.Write([]byte("Content-Type: command/reply\nReply-Text: +OK accepted\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth ok to client")

			enabledEventReq := <-actualClientRequestCh
			assert.Equal(t, "event plain MESSAGE_QUERY", enabledEventReq)

			_, err = clientConn.Write([]byte("Content-Type: command/reply\nReply-Text: +OK event listener enabled plain\r\n\r\n"))
			assert.NoError(t, err, "Cannot write command reply to client")
		}
	}()
	opts := InboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 2 * time.Second,
			Protocol:    Tcpsocket,
		},
		Network:     "tcp",
		Password:    "ClueCon",
		AuthTimeout: 2 * time.Second,
	}
	conn, err := opts.Dial(listener.Addr().String())
	require.NoError(t, err)

	res, err := conn.SendCommand(context.Background(), command.Event{
		Format: "plain",
		Listen: []string{"MESSAGE_QUERY"},
	})
	require.NoError(t, err)
	require.Len(t, res.Headers, 2)
	require.Equal(t, "command/reply", res.Headers.Get("Content-Type"))
	require.Equal(t, "+OK event listener enabled plain", res.Headers.Get("Reply-Text"))
}
