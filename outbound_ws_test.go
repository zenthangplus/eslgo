package eslgo

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testNoopHandlerConnection = func(ctx context.Context, conn *Conn, response *RawResponse) {}

func testCreateWsServer(handler OutboundHandler) (server *httptest.Server, wsUrl string) {
	opts := OutboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 5 * time.Second,
			Protocol:    Websocket,
		},
		ConnectTimeout:  1 * time.Second,
		ConnectionDelay: 25 * time.Millisecond,
	}
	muxHandler := http.NewServeMux()
	muxHandler.HandleFunc("/ws", opts.wsHandler(handler))
	server = httptest.NewServer(muxHandler)
	wsUrl = "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	return
}

func TestOutboundWS_WhenServerSendConnectCmdButClientNotReply_ShouldCloseConnection(t *testing.T) {
	server, wsUrl := testCreateWsServer(testNoopHandlerConnection)
	defer server.Close()
	wsClient, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	require.NoErrorf(t, err, "could not open a ws connection on %s", wsUrl)
	defer wsClient.Close()

	// Wait for server send connect command
	time.Sleep(100 * time.Millisecond)
	messageType, payload, err := wsClient.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "connect\r\n\r\n", string(payload))

	// Wait for server connection timeout
	time.Sleep(1100 * time.Millisecond)
	_, _, err = wsClient.ReadMessage()
	var closeError *websocket.CloseError
	isClosedErr := errors.As(err, &closeError)
	require.Equal(t, true, isClosedErr)
}

func TestOutboundWS_WhenServerSendConnectCmdAndClientReplyNotCorrectFormat_ShouldCloseConnection(t *testing.T) {
	server, wsUrl := testCreateWsServer(testNoopHandlerConnection)
	defer server.Close()
	wsClient, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	require.NoErrorf(t, err, "could not open a ws connection on %s", wsUrl)
	defer wsClient.Close()

	// Wait for server send connect command
	time.Sleep(100 * time.Millisecond)
	messageType, payload, err := wsClient.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "connect\r\n\r\n", string(payload))

	// Send connected message
	err = wsClient.WriteMessage(websocket.TextMessage, []byte("connected"))
	require.NoError(t, err)

	// Send another message to confirm that connection is established
	time.Sleep(1100 * time.Millisecond)
	_, _, err = wsClient.ReadMessage()
	var closeError *websocket.CloseError
	isClosedErr := errors.As(err, &closeError)
	require.Equal(t, true, isClosedErr)
}

func TestOutboundWS_WhenServerSendConnectCmdAndClientReplyCorrectFormat_ShouldAcceptConnection(t *testing.T) {
	server, wsUrl := testCreateWsServer(testNoopHandlerConnection)
	defer server.Close()
	wsClient, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	require.NoErrorf(t, err, "could not open a ws connection on %s", wsUrl)
	defer wsClient.Close()

	// Wait for server send connect command
	time.Sleep(100 * time.Millisecond)
	messageType, payload, err := wsClient.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "connect\r\n\r\n", string(payload))

	// Send connected message
	err = wsClient.WriteMessage(websocket.TextMessage, []byte("Content-Type: api/response\r\nContent-Length: 9\r\nUnique-Id: call-1\r\n\r\nconnected\r\n\r\n"))
	require.NoError(t, err)

	// Send another message to confirm that connection is established
	time.Sleep(1100 * time.Millisecond)
	msgType, payload, err := wsClient.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, msgType)
	require.Equal(t, "exit\r\n\r\n", string(payload)) // Exit message is sent when handler is finished
}

func TestOutboundWS_GivenServerClientConnected_WhenSendEvent_ShouldTriggerHandler(t *testing.T) {
	receivingEvent := make(chan *Event)
	handleConnection := func(ctx context.Context, conn *Conn, response *RawResponse) {
		callId := response.GetHeader("Unique-Id")
		log.Printf("Got connection for call %s, response: %#v", callId, response)

		conn.RegisterEventListener(response.GetHeader("Unique-Id"), func(event *Event) {
			log.Printf("Receive event %s for call %s. Headers: %v", event.GetName(), callId, event.Headers)
			receivingEvent <- event
		})
	}
	server, wsUrl := testCreateWsServer(handleConnection)
	defer server.Close()
	wsClient, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	require.NoErrorf(t, err, "could not open a ws connection on %s", wsUrl)
	defer wsClient.Close()

	// Wait for server send connect command
	time.Sleep(100 * time.Millisecond)
	messageType, payload, err := wsClient.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "connect\r\n\r\n", string(payload))

	// Send connected message
	err = wsClient.WriteMessage(websocket.TextMessage, []byte(`Content-Type: api/response
Content-Length: 9
Unique-Id: call-1

connected`))
	require.NoError(t, err)

	// Send an event
	time.Sleep(1100 * time.Millisecond)
	err = wsClient.WriteMessage(websocket.TextMessage, []byte(`Content-Type: text/event-plain
Content-Length: 119
Unique-Id: call-1

Content-Length: 8
Content-Type: string
Unique-Id: call-1
Test-Header: test-header1
Event-Name: CHANNEL_ANSWER

answered`))
	require.NoError(t, err)

	// Wait for handler is trigger
	select {
	case <-time.After(200 * time.Millisecond):
		require.FailNow(t, "Timeout when waiting for trigger event")
	case event := <-receivingEvent:
		assert.Equal(t, "CHANNEL_ANSWER", event.GetName())
		assert.Equal(t, "call-1", event.GetHeader("Unique-Id"))
		assert.Equal(t, "test-header1", event.GetHeader("Test-Header"))
	}
}

func TestOutboundWS_GivenClientWithRequestId_WhenServerSendConnectCmd_ShouldReturnRequestIdToHandler(t *testing.T) {
	receivingRequestId := make(chan string)
	handleConnection := func(ctx context.Context, conn *Conn, response *RawResponse) {
		callId := response.GetHeader("Unique-Id")
		log.Printf("Got connection for call %s, response: %#v", callId, response)
		receivingRequestId <- response.GetHeader(HeaderRequestId)
	}
	server, wsUrl := testCreateWsServer(handleConnection)
	defer server.Close()
	wsClient, _, err := websocket.DefaultDialer.Dial(wsUrl, http.Header{
		HeaderRequestId: []string{"request-id-1"},
	})
	require.NoErrorf(t, err, "could not open a ws connection on %s", wsUrl)
	defer wsClient.Close()

	// Wait for server send connect command
	time.Sleep(100 * time.Millisecond)
	messageType, payload, err := wsClient.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "connect\r\n\r\n", string(payload))

	// Send connected message
	err = wsClient.WriteMessage(websocket.TextMessage, []byte("Content-Type: api/response\r\nContent-Length: 9\r\nUnique-Id: call-1\r\n\r\nconnected\r\n\r\n"))
	require.NoError(t, err)

	// Wait for handler is trigger
	select {
	case <-time.After(200 * time.Millisecond):
		require.FailNow(t, "Timeout when waiting for receiving request id")
	case reqId := <-receivingRequestId:
		require.Equal(t, "request-id-1", reqId)
	}
}
