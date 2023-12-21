/*
 * Copyright (c) 2020 Percipia
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * Contributor(s):
 * Andrew Querol <aquerol@percipia.com>
 */
package eslgo

import (
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"strings"
	"time"
)

const HeaderRequestId = "X-Request-ID"

type OutboundHandler func(ctx context.Context, conn *Conn, connectResponse *RawResponse)

// OutboundOptions - Used to open a new listener for outbound ESL connections from FreeSWITCH
type OutboundOptions struct {
	Options                       // Generic common options to both Inbound and Outbound Conn
	Network         string        // The network type to listen on, should be tcp, tcp4, or tcp6
	ConnectTimeout  time.Duration // How long should we wait for FreeSWITCH to respond to our "connect" command. 5 seconds is a sane default.
	ConnectionDelay time.Duration // How long should we wait after connection to start sending commands. 25ms is the recommended default otherwise we can close the connection before FreeSWITCH finishes starting it on their end. https://github.com/signalwire/freeswitch/pull/636
}

// DefaultOutboundOptions - The default options used for creating the outbound connection
var DefaultOutboundOptions = OutboundOptions{
	Options:         DefaultOptions,
	Network:         "tcp",
	ConnectTimeout:  5 * time.Second,
	ConnectionDelay: 25 * time.Millisecond,
}

/*
 * TODO: Review if we should have a rate limiting facility to prevent DoS attacks
 * For our use it should be fine since we only want to listen on localhost
 */
// ListenAndServe - Open a new listener for outbound ESL connections from FreeSWITCH on the specified address with the provided connection handler
func ListenAndServe(address string, handler OutboundHandler) error {
	return DefaultOutboundOptions.ListenAndServe(address, handler)
}

// ListenAndServe - Open a new listener for outbound ESL connections from FreeSWITCH with provided options and handle them with the specified handler
func (opts OutboundOptions) ListenAndServe(address string, handler OutboundHandler) error {
	switch opts.Protocol {
	case Websocket:
		return opts.ListenAndServeWs(address, handler)
	case Tcpsocket:
		return opts.ListenAndServeTcp(address, handler)
	default:
		return fmt.Errorf("protocol %s not supported", opts.Protocol)
	}
}

// ListenAndServeTcp - Open a new listener to listen outbound ESL connections by Tcp socket
func (opts OutboundOptions) ListenAndServeTcp(address string, handler OutboundHandler) error {
	listener, err := net.Listen(opts.Network, address)
	if err != nil {
		return err
	}
	opts.Logger.Info("Listening for new ESL connections on %s", listener.Addr().String())
	return opts.serveTcp(listener, handler)
}

func (opts OutboundOptions) serveTcp(listener net.Listener, handler OutboundHandler) error {
	for {
		c, err := listener.Accept()
		if err != nil {
			break
		}
		conn := newConnection(NewTcpsocketConn(c), true, opts.Options)

		conn.logger.Info("New outbound connection from %s", c.RemoteAddr().String())
		go conn.dummyLoop()
		// Does not call the handler directly to ensure closing cleanly
		go conn.outboundHandle(handler, opts.ConnectionDelay, opts.ConnectTimeout, nil)
	}

	opts.Logger.Info("Outbound server shutting down")
	return errors.New("connection closed")
}

// ListenAndServeWs - Open a new listener to listen outbound ESL connections by Websocket
func (opts OutboundOptions) ListenAndServeWs(address string, handler OutboundHandler) error {
	opts.Logger.Info("Listening for new ESL Websocket connections on %s", address)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/", opts.HandleOutboundWs(handler))
	server := &http.Server{
		Addr:              address,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           mux,
	}
	return server.ListenAndServe()
}

func (opts OutboundOptions) HandleOutboundWs(handler OutboundHandler) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			opts.Logger.Error("Upgrade ws connection error: %s", err)
			return
		}
		requestId := strings.Trim(strings.TrimPrefix(r.URL.Path, "/ws"), "/")
		opts.HandleOutboundWsConn(handler, requestId)(ws)
	}
}

func (opts OutboundOptions) HandleOutboundWsConn(handler OutboundHandler, requestId string) func(ws *websocket.Conn) {
	return func(ws *websocket.Conn) {
		headers := make(map[string]string)
		if len(requestId) > 0 {
			headers[HeaderRequestId] = requestId
		}
		c := NewWebsocketConn(ws)
		conn := newConnection(c, true, opts.Options)
		conn.logger.Info("New outbound connection from %s, request id: %s", c.RemoteAddr().String(), requestId)
		go conn.dummyLoop()
		// Does not call the handler directly to ensure closing cleanly
		go conn.outboundHandle(handler, opts.ConnectionDelay, opts.ConnectTimeout, headers)
	}
}
