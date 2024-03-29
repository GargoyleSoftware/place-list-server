package main

import (
	"code.google.com/p/go.net/websocket"
	// "fmt"
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

type OutgoingCmd struct {
	Cmd    string      `json:"cmd"`
	Params interface{} `json:"params"`
}

type CreateEventParams struct {
	UserConn *websocket.Conn
	EventId  string `json:"event_id"`
	UserId   string `json:"user_id" bson:"user_id"`
}

type Event struct {
	Id       string              `json:"event_id" bson:"event_id"`
	UserId   string              `json:"user_id" bson:"user_id"`
	Upcoming map[string][]string `json:"upcoming" bson:"upcoming,omitempty"`
	Playing  string              `json:"playing" bson:"playing,omitempty"`
	History  []*PastTrack        `json:"history" bson:"history,omitempty"`
}

type Track struct {
	Votes int
	Id    string
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
	Remove  bool
}

type AddTrackParams struct {
	EventId string `json:"event_id"`
	TrackId string `json:"track_id"`
	UserId  string `json:"user_id"`
}

type StartTrackParams AddTrackParams

type SetAuthParams struct {
	UserId string `json:"user_id"`
}

/*
func (s *SocketCmd) String() string {
	return fmt.Sprintf("[cmd: %s, params: %v]", s.Cmd, s.Params)
}
*/

type ConnectionManager struct {
	Connected      []*websocket.Conn
	ConnectionLock *sync.Mutex
	CreateEvent    chan *CreateEventParams
	Upvotes        chan *UpvoteParams
	StartTrack     chan *StartTrackParams
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
		StartTrack:     make(chan *StartTrackParams),
		SetAuth:        make(chan *SetAuthParams),
		Login:          make(chan *websocket.Conn),
		Logout:         make(chan *websocket.Conn),
	}
	go m.listenForNewEvents()
	go m.listenForUpvotes()
	go m.listenForLogins()
	go m.listenForLogouts()
	go m.listenForAdds()
	go m.listenForTrackStarts()
	return m
}

func (m *ConnectionManager) listenForNewEvents() {
	for eventInfo := range m.CreateEvent {
		event, err := GetEvent(eventInfo.EventId, eventInfo.UserId)
		if err != nil {
			log.Println("FUCK BALLS")
			continue
		}
		websocket.JSON.Send(eventInfo.UserConn, &OutgoingCmd{Cmd: "event_info", Params: event})
	}
}

func (m *ConnectionManager) listenForTrackStarts() {
	events := db.C("events")
	for trackStart := range m.StartTrack {
		_, err := GetEvent(trackStart.EventId, trackStart.UserId)
		if err != nil {
			log.Println("Unable to retrieve event when starting track play: ", err.Error())
			continue
		}

        err = events.Update(bson.M{"event_id": trackStart.EventId}, bson.M{"$set": bson.M{"now_playing": trackStart.TrackId}})
		if err != nil {
            log.Println("Unable to update event when starting rack play: ", err.Error())
            continue
		}

		manager.Broadcast(&OutgoingCmd{Cmd: "start_track", Params: map[string]interface{}{"track_id": trackStart.TrackId, "event_id": trackStart.EventId}})
	}
}

func (m *ConnectionManager) listenForLogins() {
	for conn := range m.Login {
		m.Connected = append(m.Connected, conn)
	}
}

// get or create an event by id.  If you create it, you are the host.
func GetEvent(eventId string, userId string) (*Event, error) {
	c := db.C("events")
	event := new(Event)
	err := c.Find(bson.M{"event_id": eventId}).One(&event)
	if err != nil {
		if err == mgo.NotFound {
			event.Id = eventId
			event.UserId = userId
			err = c.Insert(event)
			if err != nil {
				log.Println("ERROR inserting: ", err.Error())
				return nil, err
			}
		} else {
			log.Println("ERROR querying: ", err.Error())
			return nil, err
		}
	}
	return event, nil
}

func (m *ConnectionManager) listenForAdds() {
	c := db.C("events")
	for add := range m.AddTrack {
		selector := bson.M{"event_id": add.EventId}

		var event Event
		err := c.Find(selector).One(&event)
		if err != nil {
			log.Println("ERROR querying for track: ", err.Error())
			continue
		}
		if _, ok := event.Upcoming[add.TrackId]; ok {
			continue
		}

		err = c.Update(selector, bson.M{"$addToSet": bson.M{"upcoming." + add.TrackId: add.UserId}})
		if err != nil {
			log.Println("ERROR adding track: ", err.Error())
			log.Println(selector)
			continue
		}

		err = c.Find(selector).One(&event)
		if err != nil {
			log.Println("ERROR querying for track: ", err.Error())
			continue
		}
		m.Broadcast(&OutgoingCmd{Cmd: "add_track", Params: bson.M{"track_id": add.TrackId, "upvoters": event.Upcoming[add.TrackId]}})
	}
}

func (m *ConnectionManager) listenForUpvotes() {
	c := db.C("events")
	for like := range m.Upvotes {
		selector := bson.M{"event_id": like.EventId}
		var action string
		if like.Remove {
			action = "$pull"
		} else {
			action = "$addToSet"
		}
		err := c.Update(selector, bson.M{action: bson.M{"upcoming." + like.TrackId: like.UserId}})
		if err != nil {
			log.Println("ERROR adding upvote: ", err.Error())
			log.Println(selector)
		}
		var event Event
		err = c.Find(selector).One(&event)
		if err != nil {
			log.Println("ERROR getting event info: ", err.Error())
			continue
		}
		m.Broadcast(&OutgoingCmd{Cmd: "upvote", Params: bson.M{"track_id": like.TrackId, "upvoters": event.Upcoming[like.TrackId]}})
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
				EventId: cmd.Params["event_id"].(string),
			}
		case "upvote_track":
			manager.Upvotes <- &UpvoteParams{
				UserId:  cmd.Params["user_id"].(string),
				EventId: cmd.Params["event_id"].(string),
				TrackId: cmd.Params["track_id"].(string),
				Remove:  cmd.Params["remove"].(bool),
			}
		case "start_track":
			manager.StartTrack <- &StartTrackParams{
				TrackId: cmd.Params["track_id"].(string),
				UserId:  cmd.Params["user_id"].(string),
				EventId: cmd.Params["event_id"].(string),
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
