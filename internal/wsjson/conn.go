package wsjson

import (
	"time"

	"github.com/gorilla/websocket"

	"github.com/adfoke/clioverfrp/internal/protocol"
)

func Read(conn *websocket.Conn) (protocol.Message, error) {
	var msg protocol.Message
	err := conn.ReadJSON(&msg)
	return msg, err
}

func Write(conn *websocket.Conn, msg protocol.Message) error {
	_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return conn.WriteJSON(msg)
}
