package main

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	"io"
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
	"log"
	"net/http"
	"sync"
)

var (
	db      *mgo.Database
	manager *ConnectionManager
)

type SocketCmd struct {
	Cmd    string                 `json:"cmd"`
	Client string                 `json:"client"`
	Params map[string]interface{} `json:"params"`
}

type CreateEventParams struct {
	UserConn *websocket.Conn
	EventId  string `json:"event_id"`
	UserId   string `json:"user_id" bson:"user_id"`
}

type Event struct {
	Id       string           `json:"event_id" bson:"event_id"`
	UserId   string           `json:"user_id" bson:"user_id"`
	Upcoming []*UpcomingTrack `json:"upcoming" bson:"upcoming,omitempty"`
	History  []*PastTrack     `json:"history" bson:"history,omitempty"`
}

type Track struct {
	Votes int
	Id    string
}

type StartTrackParams struct {
}

type UpcomingTrack struct {
	TrackId  string   `bson:"track_id"`
	Upvoters []string `bson:"upvoters"`
}

type PastTrack struct {
	TrackId string `bson:"track_id"`
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
	CreateEvent    chan *CreateEventParams
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
		CreateEvent:    make(chan *CreateEventParams),
		Upvotes:        make(chan *UpvoteParams),
		AddTrack:       make(chan *AddTrackParams),
		SetAuth:        make(chan *SetAuthParams),
		Login:          make(chan *websocket.Conn),
		Logout:         make(chan *websocket.Conn),
	}
	go m.listenForNewEvents()
	go m.listenForUpvotes()
	go m.listenForLogins()
	go m.listenForLogouts()
	go m.listenForAdds()
	return m
}

func (m *ConnectionManager) listenForNewEvents() {
	c := db.C("events")
	for eventInfo := range m.CreateEvent {
		event := new(Event)
		err := c.Find(bson.M{"event_id": eventInfo.EventId}).One(&event)
		if err != nil {
			if err == mgo.NotFound {
				event.Id = eventInfo.EventId
				event.UserId = eventInfo.UserId
				err = c.Insert(event)
				if err != nil {
					log.Println("ERROR inserting: ", err.Error())
				}
			} else {
                log.Println("ERROR querying: ", err.Error())
            }
		}
	}
}

func (m *ConnectionManager) listenForLogins() {
	for conn := range m.Login {
		m.Connected = append(m.Connected, conn)
	}
}

// get or create an event by id.  If you create it, you are the host.
func GetEvent(id string) {

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
        log.Println(cmd)
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
			}
		case "login":
			manager.CreateEvent <- &CreateEventParams{
				UserConn: sock,
				UserId:   cmd.Params["user_id"].(string),
				EventId:  cmd.Params["event_id"].(string),
			}
		default:
			log.Println("Didn't understand this command: ", cmd)
		}
		manager.Broadcast(cmd)
	}
	manager.Logout <- sock
}

func main() {
	s, err := mgo.Dial("localhost")
	if err != nil {
		panic(err)
	}
	db = s.DB("coca")
    manager = NewConnectionManager()

	log.Println("serving on :8080")
	http.Handle("/socket", websocket.Handler(SocketHandler))
	http.ListenAndServe(":8080", nil)
}
