package main

import (
	"context"
	"fmt"
	"github.com/zenthangplus/eslgo"
	"github.com/zenthangplus/eslgo/resource"
	"log"
	"time"
)

func main() {
	opts := eslgo.OutboundOptions{
		Options: eslgo.Options{
			Context:     context.Background(),
			Logger:      eslgo.NormalLogger{},
			ExitTimeout: 5 * time.Second,
			Protocol:    eslgo.Websocket,
		},
		Network:         "tcp",
		ConnectTimeout:  5 * time.Second,
		ConnectionDelay: 25 * time.Millisecond,
	}
	// Start listening, this is a blocking function
	log.Fatalln(opts.ListenAndServe(":8085", handleConnection))
}

func handleConnection(ctx context.Context, conn *eslgo.Conn, response *resource.RawResponse) {
	fmt.Printf("Got connection! %#v\n", response)

	// Place the call in the foreground(api) to user 100 and playback an audio file as the bLeg and no exported variables
	response, err := conn.OriginateCall(ctx, false, eslgo.Leg{CallURL: "user/100"}, eslgo.Leg{CallURL: "&playback(misc/ivr-to_hear_screaming_monkeys.wav)"}, map[string]string{})
	fmt.Println("Call Originated: ", response, err)
}
