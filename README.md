# eslgo

eslgo is a [FreeSWITCH™](https://freeswitch.com/) ESL library for GoLang that support both TCP Socket and Websocket connection from Freeswitch.
eslgo was written from the ground up in idiomatic Go for use in our production products tested handling thousands of calls per second.

## Install
```
go get github.com/zenthangplus/eslgo/v2
```
```
github.com/zenthangplus/eslgo/v2 v2.0.0
```

## Overview
- Inbound ESL Connection
- Outbound ESL Server
- Event listeners by UUID or All events
  - Unique-Id
  - Application-UUID
  - Job-UUID
- Context support for canceling requests
- All command types abstracted out
  - You can also send custom data by implementing the `Command` interface
    - `BuildMessage() string`
- Basic Helpers for common tasks
  - DTMF
  - Call origination
  - Call answer/hangup
  - Audio playback

## Examples
There are some buildable examples under the `example` directory as well
### Outbound ESL Server
```go
package main

import (
	"context"
	"fmt"
	"github.com/zenthangplus/eslgo/v2"
	"log"
)

func main() {
	// Start listening, this is a blocking function
	log.Fatalln(eslgo.ListenAndServe(":8084", handleConnection))
}

func handleConnection(ctx context.Context, conn *eslgo.Conn, response *eslgo.RawResponse) {
	fmt.Printf("Got connection! %#v\n", response)

	// Place the call in the foreground(api) to user 100 and playback an audio file as the bLeg and no exported variables
	response, err := conn.OriginateCall(ctx, false, eslgo.Leg{CallURL: "user/100"}, eslgo.Leg{CallURL: "&playback(misc/ivr-to_hear_screaming_monkeys.wav)"}, map[string]string{})
	fmt.Println("Call Originated: ", response, err)
}
```
## Inbound ESL Client
```go
package main

import (
	"context"
	"fmt"
	"github.com/zenthangplus/eslgo/v2"
	"time"
)

func main() {
	// Connect to FreeSWITCH
	conn, err := eslgo.Dial("127.0.0.1:8021", "ClueCon", func() {
		fmt.Println("Inbound Connection Disconnected")
	})
	if err != nil {
		fmt.Println("Error connecting", err)
		return
	}

	// Create a basic context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Place the call in the background(bgapi) to user 100 and playback an audio file as the bLeg and no exported variables
	response, err := conn.OriginateCall(ctx, true, eslgo.Leg{CallURL: "user/100"}, eslgo.Leg{CallURL: "&playback(misc/ivr-to_hear_screaming_monkeys.wav)"}, map[string]string{})
	fmt.Println("Call Originated: ", response, err)

	// Close the connection after sleeping for a bit
	time.Sleep(60 * time.Second)
	conn.ExitAndClose()
}
```