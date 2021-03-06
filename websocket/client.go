package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	// TODO: This is unused. Can it be removed?
	maxMessageSize = 512
)

var (
	// TODO: This is unused. Can it be removed?
	newline = []byte{'\n'}
	// TODO: This is unused. Can it be removed?
	space = []byte{' '}
)

var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Client interface {
	SessionID() string
	UserID() string
	Topics() []TopicDescriptor
	Close(error)
	HandleOutboundMessages(context.Context, chan bool)
	SendMessage(m Message) bool
}

// Compile time verification that *client implements the Client interface
var _ Client = (*client)(nil)

// Client represents a websocket connection for a single User.
// NOTE: It is a middleman between the websocket connection and the server.
type client struct {

	// sessionID for the Client, so we can tell if a user has been registered already before or not.
	sessionID string

	// The UserID for the connection.
	userID string

	// The Topics in which the user is interested.
	topics []TopicDescriptor
	// Topics map[TopicDescriptor]struct{}

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan Message
}

func NewClient(userID string, topics []TopicDescriptor, conn *websocket.Conn, send chan Message) Client {
	// topicsMap := make(map[TopicDescriptor]struct{})
	// for _, topicDescriptor := range topics {
	// 	topicsMap[topicDescriptor] = struct{}{}
	// }
	uuid := uuid.New()
	sessionID := uuid.String()
	return &client{
		sessionID: sessionID,
		userID:    userID,
		topics:    topics,
		conn:      conn,
		send:      send,
	}
}

func (c *client) SessionID() string {
	return c.sessionID
}

func (c *client) UserID() string {
	return c.userID
}

func (c *client) Topics() []TopicDescriptor {
	return c.topics
}

func (c *client) SendMessage(m Message) bool {
	select {
	case c.send <- m:
		return true
	default:
		return false
	}
}

// NOTE: I chose to do only pushing with this websocket in order to keep things lightweight.
// The following method would do something with messages coming into the server.
// // HandleInboundMessages pumps messages from the websocket connection to the server.
// //
// // The application runs readPump in a per-connection goroutine. The application
// // ensures that there is at most one reader on a connection by executing all
// // reads from this goroutine.
// func (c *client) HandleInboundMessages(server *Server) {
// 	fmt.Println("HandleInboundMessages.Start", time.Now().Format("2006-01-02 15:04:05 Z"), c.UserID())
// 	defer func() {
// 		server.UnregisterClient(c)
// 		c.conn.Close()
// 		fmt.Println("HandleInboundMessages.End", time.Now().Format("2006-01-02 15:04:05 Z"), c.UserID())
// 	}()
// 	c.conn.SetReadLimit(maxMessageSize)
// 	c.conn.SetReadDeadline(time.Now().Add(pongWait))
// 	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
// 	for {
// 		_, byteSlice, err := c.conn.ReadMessage()
// 		if err != nil {
// 			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
// 				log.Printf("error: %v", err)
// 			}
// 			break
// 		}
// 		byteSlice = bytes.TrimSpace(bytes.Replace(byteSlice, newline, space, -1))
// 		message := Message{Text: string(byteSlice)}
// 		fmt.Println("InboundMessage", time.Now().Format("2006-01-02 15:04:05 Z"), c.UserID(), message)
// 		message.Text = "Heyoo: " + message.Text
// 		server.broadcastChan <- message
// 		// DO SOMETHNG WITH INCOMING MESSAGE HERE.
// 	}
// }

func (c *client) Close(err error) {
	errBytes := []byte{}
	if err != nil {
		errBytes = []byte(err.Error())
	}
	logrus.WithError(err).WithFields(logrus.Fields{
		"UserID":    c.UserID(),
		"SesisonID": c.SessionID(),
	}).Infof("closing client")
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage, errBytes)
		c.conn.Close()
	}
	close(c.send)
}

// HandleOutboundMessages pumps messages from the server to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *client) HandleOutboundMessages(ctx context.Context, readyChan chan bool) {
	logEntry := logrus.WithFields(logrus.Fields{
		"client.UserID":    c.UserID(),
		"client.SessionID": c.SessionID(),
	})
	logEntry.Infof("HandleOutboundMessages:Start")
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		logEntry.Infof("HandleOutboundMessages:End")
		// When we exit this goroutine, then we close the client and unregister it.
		ticker.Stop()
	}()
	readyChan <- true
	for {
		select {

		// [CASE] Context was cancelled, and we need to bail!
		case <-ctx.Done():
			logEntry.Infof("context cancelled; quitting client HandleOutboundMessages goroutine")
			return

		// [CASE] Receive a message from the Server to convey to the Client's Connection.
		case message, ok := <-c.send:
			logEntry.WithFields(logrus.Fields{
				"message": message,
				"ok":      ok,
			}).Infof("client: send")
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				return
			}

			numExtraMessages := len(c.send)
			messages := make([]Message, numExtraMessages+1)
			messages[0] = message
			for i := 1; i == numExtraMessages; i++ {
				extraMessage, ok := <-c.send
				if ok {
					messages[i] = extraMessage
				}
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			j, err := json.MarshalIndent(messages, "", "  ")
			if err != nil {
				return
			}
			logEntry.WithFields(logrus.Fields{
				"client.UserID": c.UserID(),
				"json":          string(j),
			}).Infof("HandleOutboundMessages:OutboundMessages")
			w.Write([]byte(j))

			if err := w.Close(); err != nil {
				return
			}

		// [CASE] Send a Ping. Are you still there?
		case <-ticker.C:
			logEntry.Infof("HandleOutboundMessages:Ping")
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
