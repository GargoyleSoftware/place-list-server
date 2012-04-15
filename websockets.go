package main

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
)

var manager = NewConnectionManager()

type SocketCmd struct {
	Cmd    string                 `json:"cmd"`
	Client string                 `json:"client"`
	Params map[string]interface{} `json:"params"`
}

type Event struct {
	Id string `json:"id"`
}

type Track struct {
	Votes int
	Id    string
}

type UpvoteParams struct {
	UserId  string
	EventId string
	TrackId string
	Power   int
}

type AddTrackParams struct {
	TrackId string `json:"track_id"`
	UserId  string `json:"user_id"`
}

type SetAuthParams struct {
	UserId string `json:"user_id"`
}

func (s *SocketCmd) String() string {
	return fmt.Sprintf("[cmd: %s, params: %v]", s.Cmd, s.Params)
}

type ConnectionManager struct {
	Connected      []*websocket.Conn
	ConnectionLock *sync.Mutex
	Upvotes        chan *UpvoteParams
	AddTrack       chan *AddTrackParams
	SetAuth        chan *SetAuthParams
	Login          chan *websocket.Conn
	Logout         chan *websocket.Conn
}

func NewConnectionManager() *ConnectionManager {
	m := &ConnectionManager{
		Connected:      []*websocket.Conn{},
		ConnectionLock: new(sync.Mutex),
		Upvotes:        make(chan *UpvoteParams),
		AddTrack:       make(chan *AddTrackParams),
		SetAuth:        make(chan *SetAuthParams),
		Login:          make(chan *websocket.Conn),
		Logout:         make(chan *websocket.Conn),
	}
	go m.listenForUpvotes()
	go m.listenForLogins()
	go m.listenForLogouts()
	go m.listenForAdds()
	return m
}

func (m *ConnectionManager) listenForLogins() {
	for conn := range m.Login {
		m.Connected = append(m.Connected, conn)
	}
}

func (m *ConnectionManager) listenForAdds() {
	for add := range m.AddTrack {
		m.Broadcast(add)
	}
}

func (m *ConnectionManager) listenForUpvotes() {
	for like := range m.Upvotes {
		m.Broadcast(like)
	}
}

func (m *ConnectionManager) listenForLogouts() {
	for conn := range m.Logout {
		for i, other := range m.Connected {
			if conn == other {
				m.Connected = append(m.Connected[:i], m.Connected[i:len(m.Connected)-1]...)
				log.Println("Found matching connection, breaking.")
				return
			}
		}
	}
}

func (m *ConnectionManager) Broadcast(v interface{}) {
	m.ConnectionLock.Lock()
	for _, conn := range m.Connected {
		websocket.JSON.Send(conn, v)
	}
	m.ConnectionLock.Unlock()
}

func SocketHandler(sock *websocket.Conn) {
	log.Println("Added new websocket connection ++++++++++++++++++++++++++++++++++++++++")
	manager.Login <- sock
	var cmd SocketCmd
	for {
		err := websocket.JSON.Receive(sock, &cmd)
		if err != nil {
			if err == io.EOF {
				log.Println("User disconnected ----------------------------------------")
			} else {
				log.Println("ERROR:", err.Error())
			}
			break
		}
		switch cmd.Cmd {
		case "add_track":
			manager.AddTrack <- &AddTrackParams{
				TrackId: cmd.Params["track_id"].(string),
				UserId:  cmd.Params["user_id"].(string),
			}
		case "upvote_track":
			manager.Upvotes <- &UpvoteParams{
				UserId:  cmd.Params["user_id"].(string),
				EventId: cmd.Params["event_id"].(string),
				TrackId: cmd.Params["track_id"].(string),
				Power:   cmd.Params["power"].(int),
			}
		default:
			log.Println("Didn't understand this command: ", cmd)
		}
		manager.Broadcast(cmd)
	}
	manager.Logout <- sock
}

func main() {
	http.Handle("/socket", websocket.Handler(SocketHandler))
	http.ListenAndServe(":8080", nil)
}
