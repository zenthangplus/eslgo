package eslgo

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zenthangplus/eslgo/v2/command"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func createTestWsServerForInbound(t *testing.T) (server *httptest.Server, wsUrl string, connectionCh chan *websocket.Conn) {
	connectionCh = make(chan *websocket.Conn)
	muxHandler := http.NewServeMux()
	muxHandler.HandleFunc("/ws", createTestWsHandlerForInbound(t, connectionCh))
	server = httptest.NewServer(muxHandler)
	wsUrl = "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	return
}

func createTestWsHandlerForInbound(t *testing.T, connectionCh chan *websocket.Conn) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := &websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		connectionCh <- ws
	}
}

func createTestWsResponseHandlerForInbound(t *testing.T, conn *websocket.Conn, actualClientRequest chan string) {
	for {
		messageType, msg, err := conn.ReadMessage()
		if websocket.IsCloseError(err,
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseProtocolError,
			websocket.CloseUnsupportedData,
			websocket.CloseNoStatusReceived,
			websocket.CloseAbnormalClosure,
			websocket.CloseInvalidFramePayloadData,
			websocket.ClosePolicyViolation,
			websocket.CloseMessageTooBig,
			websocket.CloseMandatoryExtension,
			websocket.CloseInternalServerErr,
			websocket.CloseServiceRestart,
			websocket.CloseTryAgainLater,
			websocket.CloseTLSHandshake) {
			break
		}
		require.NoError(t, err)
		require.Equal(t, websocket.TextMessage, messageType)
		actualClientRequest <- string(msg)
	}
}

func TestInboundWs_WhenClientAuthenButServerNotReplyAuthStatus_ShouldCloseConnection(t *testing.T) {
	server, wsUrl, connectionCh := createTestWsServerForInbound(t)
	defer server.Close()

	var clientConn *websocket.Conn
	var actualClientRequestCh = make(chan string)
	go func() {
		select {
		case <-time.After(5 * time.Second):
			require.FailNow(t, "No incoming connection found")
			break
		case clientConn = <-connectionCh:
			go createTestWsResponseHandlerForInbound(t, clientConn, actualClientRequestCh)

			err := clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: auth/request\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth/request to client")

			authReq := <-actualClientRequestCh
			assert.Equal(t, "auth ClueCon\r\n\r\n", authReq)

			exitReq := <-actualClientRequestCh
			assert.Equal(t, "exit\r\n\r\n", exitReq)

			err = clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: command/reply\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write exit reply to client")
		}
	}()
	opts := InboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 2 * time.Second,
			Protocol:    Websocket,
		},
		Network:     "tcp",
		Password:    "ClueCon",
		AuthTimeout: 2 * time.Second,
	}
	_, err := opts.Dial(wsUrl)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestInboundWs_WhenClientAuthenButServerReplyAuthenFailed_ShouldCloseConnection(t *testing.T) {
	server, wsUrl, connectionCh := createTestWsServerForInbound(t)
	defer server.Close()

	var clientConn *websocket.Conn
	var actualClientRequestCh = make(chan string)
	go func() {
		select {
		case <-time.After(5 * time.Second):
			require.FailNow(t, "No incoming connection found")
			break
		case clientConn = <-connectionCh:
			go createTestWsResponseHandlerForInbound(t, clientConn, actualClientRequestCh)

			err := clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: auth/request\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth/request to client")

			authReq := <-actualClientRequestCh
			assert.Equal(t, "auth ClueCon\r\n\r\n", authReq)

			err = clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: command/reply\nReply-Text: -ERR invalid\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth failed to client")

			exitReq := <-actualClientRequestCh
			assert.Equal(t, "exit\r\n\r\n", exitReq)

			err = clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: command/reply\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write exit reply to client")
		}
	}()
	opts := InboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 2 * time.Second,
			Protocol:    Websocket,
		},
		Network:     "tcp",
		Password:    "ClueCon",
		AuthTimeout: 2 * time.Second,
	}
	_, err := opts.Dial(wsUrl)
	require.Equal(t, 0, strings.Index(err.Error(), "failed to auth"), "Error should start with 'failed to auth'")
}

func TestInboundWs_WhenClientAuthenButServerReplyAuthenOk_ShouldEstablishedConnection(t *testing.T) {
	server, wsUrl, connectionCh := createTestWsServerForInbound(t)
	defer server.Close()

	var clientConn *websocket.Conn
	var actualClientRequestCh = make(chan string)
	go func() {
		select {
		case <-time.After(5 * time.Second):
			require.FailNow(t, "No incoming connection found")
			break
		case clientConn = <-connectionCh:
			go createTestWsResponseHandlerForInbound(t, clientConn, actualClientRequestCh)

			err := clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: auth/request\r\nContent-Length: 0\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth/request to client")

			authReq := <-actualClientRequestCh
			assert.Equal(t, "auth ClueCon\r\n\r\n", authReq)

			err = clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: command/reply\nReply-Text: +OK accepted\r\n\r\n"))
			assert.NoError(t, err, "Cannot write auth ok to client")

			enabledEventReq := <-actualClientRequestCh
			assert.Equal(t, "event plain MESSAGE_QUERY\r\n\r\n", enabledEventReq)

			err = clientConn.WriteMessage(websocket.TextMessage, []byte("Content-Type: command/reply\nReply-Text: +OK event listener enabled plain\r\n\r\n"))
			assert.NoError(t, err, "Cannot write command reply to client")
		}
	}()
	opts := InboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 2 * time.Second,
			Protocol:    Websocket,
		},
		Network:     "tcp",
		Password:    "ClueCon",
		AuthTimeout: 2 * time.Second,
	}
	conn, err := opts.Dial(wsUrl)
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
