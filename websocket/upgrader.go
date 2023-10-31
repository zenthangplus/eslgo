package websocket

import (
	"github.com/gorilla/websocket"
	"net/http"
)

func NewUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
}
