package eslgo

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"
)

func testCreateTcpServer(t *testing.T, handler OutboundHandler) (listener net.Listener) {
	serverAddr := ":0"
	opts := OutboundOptions{
		Options: Options{
			Context:     context.Background(),
			Logger:      NormalLogger{},
			ExitTimeout: 5 * time.Second,
			Protocol:    Tcpsocket,
		},
		Network:         "tcp",
		ConnectTimeout:  1 * time.Second,
		ConnectionDelay: 25 * time.Millisecond,
	}
	listener, err := net.Listen(opts.Network, serverAddr)
	if err != nil {
		require.NoError(t, err, "Cannot create listener for tcp server")
	}
	go opts.serveTcp(listener, handler)
	return listener
}

func TestOutboundTcp_WhenServerSendConnectCmdButClientNotReply_ShouldCloseConnection(t *testing.T) {
	listener := testCreateTcpServer(t, testNoopHandlerConnection)
	defer listener.Close()

	// Connect to the server and send a message
	time.Sleep(100 * time.Millisecond)
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoErrorf(t, err, "Cannot connect to tcp server: %s", listener.Addr().String())
	defer conn.Close()

	// Check for server send `connect` message.
	// Note: Length of `connect` message is 7, but we want to consume all other newline chars.
	actual := make([]byte, 11)
	_, err = conn.Read(actual)
	require.NoError(t, err)
	require.Equal(t, "connect", strings.TrimSpace(string(actual)))

	// Wait for server connection timeout
	time.Sleep(1100 * time.Millisecond)
	err = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	require.NoError(t, err)
	actual = make([]byte, 1)
	_, err = conn.Read(actual)
	require.ErrorIs(t, err, io.EOF) // connection closed
}

func TestOutboundTcp_WhenServerSendConnectCmdAndClientReplyNotCorrectFormat_ShouldCloseConnection(t *testing.T) {
	listener := testCreateTcpServer(t, testNoopHandlerConnection)
	defer listener.Close()

	// Connect to the server and send a message
	time.Sleep(100 * time.Millisecond)
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoErrorf(t, err, "Cannot connect to tcp server: %s", listener.Addr().String())
	defer conn.Close()

	// Check for server send `connect` message.
	// Note: Length of `connect` message is 7, but we want to consume all other newline chars.
	actual := make([]byte, 11)
	_, err = conn.Read(actual)
	require.NoError(t, err)
	require.Equal(t, "connect", strings.TrimSpace(string(actual)))

	// Send connected message
	_, err = conn.Write([]byte("connected"))
	require.NoError(t, err)

	// Wait for server connection timeout
	time.Sleep(1100 * time.Millisecond)
	err = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	require.NoError(t, err)
	actual = make([]byte, 1)
	_, err = conn.Read(actual)
	require.ErrorIs(t, err, io.EOF) // connection closed
}

func TestOutboundTcp_WhenServerSendConnectCmdAndClientReplyCorrectFormat_ShouldAcceptConnection(t *testing.T) {
	listener := testCreateTcpServer(t, testNoopHandlerConnection)
	defer listener.Close()

	// Connect to the server and send a message
	time.Sleep(100 * time.Millisecond)
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoErrorf(t, err, "Cannot connect to tcp server: %s", listener.Addr().String())
	defer conn.Close()

	// Check for server send `connect` message.
	// Note: Length of `connect` message is 7, but we want to consume all other newline chars.
	actual := make([]byte, 11)
	_, err = conn.Read(actual)
	require.NoError(t, err)
	require.Equal(t, "connect", strings.TrimSpace(string(actual)))

	// Send connected message
	_, err = conn.Write([]byte(`Content-Type: api/response
Content-Length: 9
Unique-Id: call-1

connected`))
	require.NoError(t, err)

	// Wait longer then timeout of server
	time.Sleep(1100 * time.Millisecond)
	err = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	require.NoError(t, err)
	actual = make([]byte, 4)
	_, err = conn.Read(actual)
	require.NoError(t, err)
	assert.Equal(t, "exit", string(actual)) // Exit message is sent when handler is finished
}

func TestOutboundTcp_GivenServerClientConnected_WhenSendEvent_ShouldTriggerHandler(t *testing.T) {
	receivingEvent := make(chan *Event)
	handleConnection := func(ctx context.Context, conn *Conn, response *RawResponse) {
		callId := response.GetHeader("Unique-Id")
		log.Printf("Got connection for call %s, response: %#v", callId, response)

		conn.RegisterEventListener(response.GetHeader("Unique-Id"), func(event *Event) {
			log.Printf("Receive event %s for call %s. Headers: %v", event.GetName(), callId, event.Headers)
			receivingEvent <- event
		})
	}
	listener := testCreateTcpServer(t, handleConnection)
	defer listener.Close()

	// Connect to the server and send a message
	time.Sleep(100 * time.Millisecond)
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoErrorf(t, err, "Cannot connect to tcp server: %s", listener.Addr().String())
	defer conn.Close()

	// Check for server send `connect` message.
	// Note: Length of `connect` message is 7, but we want to consume all other newline chars.
	actual := make([]byte, 11)
	_, err = conn.Read(actual)
	require.NoError(t, err)
	require.Equal(t, "connect", strings.TrimSpace(string(actual)))

	// Send connected message
	_, err = conn.Write([]byte(`Content-Type: api/response
Content-Length: 9
Unique-Id: call-1

connected`))
	require.NoError(t, err)

	// Send an event
	time.Sleep(1100 * time.Millisecond)
	_, err = conn.Write([]byte(`Content-Type: text/event-plain
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
