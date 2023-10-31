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
	"github.com/google/uuid"
	"github.com/zenthangplus/eslgo/command"
	"github.com/zenthangplus/eslgo/resource"
	"sync"
	"time"
)

type Conn struct {
	conn              FsConn
	writeLock         sync.Mutex
	runningContext    context.Context
	stopFunc          func()
	responseChannels  map[string]chan *resource.RawResponse
	responseChanMutex sync.RWMutex
	eventListenerLock sync.RWMutex
	eventListeners    map[string]map[string]EventListener
	outbound          bool
	logger            Logger
	exitTimeout       time.Duration
	closeOnce         sync.Once
	closeDelay        time.Duration
}

// Options - Generic options for an ESL connection, either inbound or outbound
type Options struct {
	Context     context.Context // This specifies the base running context for the connection. If this context expires all connections will be terminated.
	Logger      Logger          // This specifies the logger to be used for any library internal messages. Can be set to nil to suppress everything.
	ExitTimeout time.Duration   // How long should we wait for FreeSWITCH to respond to our "exit" command. 5 seconds is a sane default.
	Protocol    Protocol
}

// DefaultOptions - The default options used for creating the connection
var DefaultOptions = Options{
	Context:     context.Background(),
	Logger:      NormalLogger{},
	ExitTimeout: 5 * time.Second,
	Protocol:    Tcpsocket,
}

func newConnection(c FsConn, outbound bool, opts Options) *Conn {
	// If logger is nil, do not actually output anything
	if opts.Logger == nil {
		opts.Logger = NilLogger{}
	}

	runningContext, stop := context.WithCancel(opts.Context)

	instance := &Conn{
		conn: c,
		responseChannels: map[string]chan *resource.RawResponse{
			resource.TypeReply:       make(chan *resource.RawResponse),
			resource.TypeAPIResponse: make(chan *resource.RawResponse),
			resource.TypeEventPlain:  make(chan *resource.RawResponse),
			resource.TypeEventXML:    make(chan *resource.RawResponse),
			resource.TypeEventJSON:   make(chan *resource.RawResponse),
			resource.TypeAuthRequest: make(chan *resource.RawResponse, 1), // Buffered to ensure we do not lose the initial auth request before we are setup to respond
			resource.TypeDisconnect:  make(chan *resource.RawResponse),
		},
		runningContext: runningContext,
		stopFunc:       stop,
		eventListeners: make(map[string]map[string]EventListener),
		outbound:       outbound,
		logger:         opts.Logger,
		exitTimeout:    opts.ExitTimeout,
	}
	go instance.receiveLoop()
	go instance.eventLoop()
	return instance
}

// RegisterEventListener - Registers a new event listener for the specified channel UUID(or EventListenAll). Returns the registered listener ID used to remove it.
func (c *Conn) RegisterEventListener(channelUUID string, listener EventListener) string {
	c.eventListenerLock.Lock()
	defer c.eventListenerLock.Unlock()

	id := uuid.New().String()
	if _, ok := c.eventListeners[channelUUID]; ok {
		c.eventListeners[channelUUID][id] = listener
	} else {
		c.eventListeners[channelUUID] = map[string]EventListener{id: listener}
	}
	return id
}

// RemoveEventListener - Removes the listener for the specified channel UUID with the listener ID returned from RegisterEventListener
func (c *Conn) RemoveEventListener(channelUUID string, id string) {
	c.eventListenerLock.Lock()
	defer c.eventListenerLock.Unlock()

	if listeners, ok := c.eventListeners[channelUUID]; ok {
		delete(listeners, id)
	}
}

// SendCommand - Sends the specified ESL command to FreeSWITCH with the provided context. Returns the response data and any errors encountered.
func (c *Conn) SendCommand(ctx context.Context, cmd command.Command) (*resource.RawResponse, error) {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	if linger, ok := cmd.(command.Linger); ok {
		if linger.Enabled {
			if linger.Seconds > 0 {
				c.closeDelay = linger.Seconds
			} else {
				c.closeDelay = -1
			}
		} else {
			c.closeDelay = 0
		}
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	}
	err := c.conn.Write(cmd.BuildMessage())
	if err != nil {
		return nil, err
	}

	// Get response
	c.responseChanMutex.RLock()
	defer c.responseChanMutex.RUnlock()
	select {
	case response := <-c.responseChannels[resource.TypeReply]:
		if response == nil {
			// We only get nil here if the channel is closed
			return nil, errors.New("connection closed")
		}
		return response, nil
	case response := <-c.responseChannels[resource.TypeAPIResponse]:
		if response == nil {
			// We only get nil here if the channel is closed
			return nil, errors.New("connection closed")
		}
		return response, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ExitAndClose - Attempt to gracefully send FreeSWITCH "exit" over the ESL connection before closing our connection and stopping. Protected by a sync.Once
func (c *Conn) ExitAndClose() {
	c.closeOnce.Do(func() {
		// Attempt a graceful closing of the connection with FreeSWITCH
		ctx, cancel := context.WithTimeout(c.runningContext, c.exitTimeout)
		_, _ = c.SendCommand(ctx, command.Exit{})
		cancel()
		c.close()
	})
}

// Close - Close our connection to FreeSWITCH without sending "exit". Protected by a sync.Once
func (c *Conn) Close() {
	c.closeOnce.Do(c.close)
}

func (c *Conn) close() {
	// Allow users to do anything they need to do before we tear everything down
	c.stopFunc()
	c.responseChanMutex.Lock()
	defer c.responseChanMutex.Unlock()
	for key, responseChan := range c.responseChannels {
		close(responseChan)
		delete(c.responseChannels, key)
	}

	// Close the connection only after we have the response channel lock and we have deleted all response channels to ensure we don't receive on a closed channel
	_ = c.conn.Close()
}

func (c *Conn) callEventListener(event *Event) {
	c.eventListenerLock.RLock()
	defer c.eventListenerLock.RUnlock()

	// First check if there are any general event listener
	if listeners, ok := c.eventListeners[EventListenAll]; ok {
		for _, listener := range listeners {
			go listener(event)
		}
	}

	// Next call any listeners for a particular channel
	if event.HasHeader("Unique-Id") {
		channelUUID := event.GetHeader("Unique-Id")
		if listeners, ok := c.eventListeners[channelUUID]; ok {
			for _, listener := range listeners {
				go listener(event)
			}
		}
	}

	// Next call any listeners for a particular application
	if event.HasHeader("Application-UUID") {
		appUUID := event.GetHeader("Application-UUID")
		if listeners, ok := c.eventListeners[appUUID]; ok {
			for _, listener := range listeners {
				go listener(event)
			}
		}
	}

	// Next call any listeners for a particular job
	if event.HasHeader("Job-UUID") {
		jobUUID := event.GetHeader("Job-UUID")
		if listeners, ok := c.eventListeners[jobUUID]; ok {
			for _, listener := range listeners {
				go listener(event)
			}
		}
	}
}

func (c *Conn) eventLoop() {
	for {
		var event *Event
		var err error
		c.responseChanMutex.RLock()
		select {
		case raw := <-c.responseChannels[resource.TypeEventPlain]:
			if raw == nil {
				// We only get nil here if the channel is closed
				c.responseChanMutex.RUnlock()
				return
			}
			event, err = readPlainEvent(raw.Body)
		case raw := <-c.responseChannels[resource.TypeEventXML]:
			if raw == nil {
				// We only get nil here if the channel is closed
				c.responseChanMutex.RUnlock()
				return
			}
			event, err = readXMLEvent(raw.Body)
		case raw := <-c.responseChannels[resource.TypeEventJSON]:
			if raw == nil {
				// We only get nil here if the channel is closed
				c.responseChanMutex.RUnlock()
				return
			}
			event, err = readJSONEvent(raw.Body)
		case <-c.runningContext.Done():
			c.responseChanMutex.RUnlock()
			return
		}
		c.responseChanMutex.RUnlock()

		if err != nil {
			c.logger.Warn("Parsing event error: %s", err.Error())
			continue
		}

		c.callEventListener(event)
	}
}

func (c *Conn) receiveLoop() {
	for c.runningContext.Err() == nil {
		err := c.doMessage()
		if err != nil {
			c.logger.Warn("Error receiving message: %s", err.Error())
			break
		}
	}
}

func (c *Conn) doMessage() error {
	response, err := c.conn.ReadResponse()
	if err != nil {
		return err
	}

	c.responseChanMutex.RLock()
	defer c.responseChanMutex.RUnlock()
	responseChan, ok := c.responseChannels[response.GetHeader("Content-Type")]
	if !ok && len(c.responseChannels) <= 0 {
		// We must have shutdown!
		return errors.New("no response channels")
	}

	// We have a handler
	if ok {
		// Only allow 5 seconds to allow the handler to receive hte message on the channel
		ctx, cancel := context.WithTimeout(c.runningContext, 5*time.Second)
		defer cancel()

		select {
		case responseChan <- response:
		case <-c.runningContext.Done():
			// Parent connection context has stopped we most likely shutdown in the middle of waiting for a handler to handle the message
			return c.runningContext.Err()
		case <-ctx.Done():
			// Do not return an error since this is not fatal but log since it could be a indication of problems
			c.logger.Warn("No one to handle response. Is the connection overloaded or stopping? Response: %v", response)
		}
	} else {
		return errors.New("no response channel for Content-Type: " + response.GetHeader("Content-Type"))
	}
	return nil
}

func (c *Conn) outboundHandle(handler OutboundHandler, connectionDelay, connectTimeout time.Duration) {
	ctx, cancel := context.WithTimeout(c.runningContext, connectTimeout)
	response, err := c.SendCommand(ctx, command.Connect{})
	cancel()
	if err != nil {
		c.logger.Warn("Error connecting to %s error %s", c.conn.RemoteAddr().String(), err.Error())
		// Try closing cleanly first
		c.Close() // Not ExitAndClose since this error connection is most likely from communication failure
		return
	}
	handler(c.runningContext, c, response)
	// XXX This is ugly, the issue with short lived async sockets on our end is if they complete too fast we can actually
	// close the connection before FreeSWITCH is in a state to close the connection on their end. 25ms is an magic value
	// found by testing to have no failures on my test system. I started at 1 second and reduced as far as I could go.
	// TODO This actually may be fixed: https://github.com/signalwire/freeswitch/pull/636
	time.Sleep(connectionDelay)
	c.ExitAndClose()
}

func (c *Conn) dummyLoop() {
	select {
	case <-c.responseChannels[resource.TypeDisconnect]:
		c.logger.Info("Disconnect outbound connection", c.conn.RemoteAddr())
		if c.closeDelay >= 0 {
			time.AfterFunc(c.closeDelay, func() {
				c.Close()
			})
		}
	case <-c.responseChannels[resource.TypeAuthRequest]:
		c.logger.Debug("Ignoring auth request on outbound connection", c.conn.RemoteAddr())
	case <-c.runningContext.Done():
		return
	}
}
