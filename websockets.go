package main

import (
	"code.google.com/p/go.net/websocket"
	"log"
    "fmt"
	"net/http"
    "regexp"
)

var trackPattern = regexp.MustCompile("spotify:track:.*")

type SocketCmd struct {
	Cmd    string      `json:"cmd"`
	Params interface{} `json:"params"`
}

func (s *SocketCmd) String() string {
    return fmt.Sprintf("[cmd: %s, params: %v]", s.Cmd, s.Params)
}

type AddTrackParams struct {
	Trackname string `json:"trackname"`
}

func SocketHandler(sock *websocket.Conn) {
	var cmd SocketCmd
	for {
        websocket.JSON.Receive(sock, &cmd)
        log.Println(cmd)
	}
}

func main() {
	http.Handle("/socket", websocket.Handler(SocketHandler))
	http.ListenAndServe(":8080", nil)
}
