package websocket

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var errUndeliverableClient = fmt.Errorf("message was undeliverable to client")
var errUnregistrationRequest = fmt.Errorf("an outside process unregistered the client")
var errAlreadyRegistered = fmt.Errorf("UserID has been re-registered")
var errServerShutdown = fmt.Errorf("Server is shutting down")

const SystemUserID = "SYSTEM"

var emptyTopic = TopicDescriptor{}
var systemTopic = TopicDescriptor{
	Type: TopicTypeSystem,
}

// Server maintains the set of active clients and broadcasts messages to the clients.
type Server struct {
	mutex sync.RWMutex

	// Registered clientsByUserID & mutex.
	clientsByUserID map[string]*Client

	// Registered userIDsByTopicDescriptor & mutex.
	userIDsByTopicDescriptor map[TopicDescriptor]map[string]struct{}

	// Inbound messages from the clients.
	broadcastChan chan MessageForBroadcast

	// Register requests from the clients.
	registerClientChan chan *Client

	// Unregister requests from clients after each Message is sent.
	unregisterClientChan chan *Client

	_isClosed bool
}

// NewServer returns a new Server.
func NewServer() *Server {
	userIDsByTopicDescriptor := make(map[TopicDescriptor]map[string]struct{})
	userIDsByTopicDescriptor[systemTopic] = make(map[string]struct{})
	return &Server{
		clientsByUserID:          make(map[string]*Client),
		userIDsByTopicDescriptor: userIDsByTopicDescriptor,
		broadcastChan:            make(chan MessageForBroadcast),
		registerClientChan:       make(chan *Client),
		unregisterClientChan:     make(chan *Client),
	}
}

// RegisterClient should be called by system resources to check if a Topic declared by any Clients.
func (server *Server) IsClosed() bool {
	return server._isClosed
}

// RegisterClient should be called by system resources to check if a Topic declared by any Clients.
func (server *Server) Close() {
	if !server._isClosed {
		server._isClosed = true
		for _, client := range server.clientsByUserID {
			actuateUnregisterClient(server, client, errServerShutdown)
		}
		close(server.registerClientChan)
		close(server.unregisterClientChan)
		close(server.broadcastChan)
	}
}

// RegisterClient should be called by system resources to check if a Topic declared by any Clients.
func (server *Server) TopicIsRegistered(topic TopicDescriptor) bool {
	if server._isClosed {
		return false
	}
	_, ok := server.userIDsByTopicDescriptor[topic]
	return ok
}

// RegisterClient should be called by system resources to Register Client(s).
func (server *Server) RegisterClient(client *Client) {
	if server._isClosed {
		return
	}
	logrus.WithFields(logrus.Fields{
		"client.UserID":    client.UserID,
		"client.SessionID": client.SessionID(),
	}).Infof("RegisterClient")
	// Send this client into the `registerClientChan`. The receiver will register the Client for the declared Topics.
	server.registerClientChan <- client
}

// UnregisterClient should be called by system resources to Unregister Client(s).
func (server *Server) UnregisterClient(client *Client) {
	if server._isClosed {
		return
	}
	server.unregisterClientChan <- client
}

// BroadcastMessage should be called by system resources to send messages to Client(s).
func (server *Server) BroadcastMessage(bm MessageForBroadcast) error {
	if server._isClosed {
		return errServerShutdown
	}

	if bm.Topic == emptyTopic {
		return fmt.Errorf("failed to broadcast message from '%s' with no topic", bm.FromUserID)
	}

	// If this message came from a user, validate the SessionID!
	if len(bm.FromSessionID) > 0 {

		// Confirm sending user has a client registered.
		server.mutex.Lock()
		fromClient, fromClientIsRegistered := server.clientsByUserID[bm.FromUserID]
		server.mutex.Unlock()
		if !fromClientIsRegistered || fromClient == nil {
			return fmt.Errorf("failed to broadcast message since user '%s' has no registered client", bm.FromUserID)
		}

		// Confirm message is from the correct sessionID.
		if bm.FromSessionID != fromClient.SessionID() {
			return fmt.Errorf("failed to broadcast message since it came from an invalid session")
		}
	}

	// Validate the server has this topic.
	if topicIsRegistered := server.TopicIsRegistered(bm.Topic); !topicIsRegistered {
		return fmt.Errorf("failed to broadcast message since topic '%s' has not been declared", bm.Message.Topic)
	}

	// If there was a client found, ensure that it is subscribed to the topic being messaged upon.
	// For now, users are permitted to generate system messages for demonstration purposes only.
	// NOTE: This should be disallowed in a real system.
	foundTopic := false
	if bm.Topic.Type == TopicTypeSystem || bm.Topic.Type == TopicTypeAuth {

		// No Validation; let it fly.
	} else {

		// Confirmreceiving user has a client registered.
		server.mutex.Lock()
		toClient, toClientIsRegistered := server.clientsByUserID[bm.FromUserID]
		server.mutex.Unlock()
		if !toClientIsRegistered || toClient == nil {
			return fmt.Errorf("failed to broadcast message since user '%s' has no registered client", bm.FromUserID)
		}

		// Confirm user is subscribed on the Topic.
		for _, topicDescriptor := range toClient.Topics {
			if topicDescriptor == bm.Message.Topic {
				foundTopic = true
				break
			}
		}
		if !foundTopic {
			return fmt.Errorf("failed to broadcast message since user has not declared topic '%s'", bm.Message.Topic)
		}
	}
	logrus.WithFields(logrus.Fields{
		"bm.FromUserID":        bm.FromUserID,
		"bm.ToUserID":          bm.ToUserID,
		"message":              bm.Message,
		"foundTopic":           foundTopic,
		"len(s.broadcastChan)": len(server.broadcastChan),
	}).Infof("s.broadcastChan <- bm.Message")

	// Send this message to be broadcasted. The receiver will send it to everybody on the topic.
	server.broadcastChan <- bm
	return nil
}

// Run should be ran on a goroutine, and it should not be called more than once.
func (server *Server) Run(ctx context.Context) {
	if server._isClosed {
		return
	}

	// Start a ticker and defer closing a few things on the way out.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer func() {
		ticker.Stop()
		server.Close()
	}()

	// Infinite loop w/ a heartbeat.
	for {
		// Select the first channel that has a value in it.
		select {

		// [CASE] Context was cancelled, and we need to bail!
		case <-ctx.Done():
			logrus.Infof("context cancelled; quitting server")
			return

		// [CASE] Should we Register this Client?
		// Unregister & Re-register Client.
		case client := <-server.registerClientChan:
			go actuateRegisterClient(ctx, server, client)

		// [CASE] Should we Unregister this Client?
		case client := <-server.unregisterClientChan:
			go actuateUnregisterClient(server, client, errUnregistrationRequest)

		// [CASE] Do we have a message to send?
		// Send messages to potential targets:
		// - Single user
		// - Multiple users
		// - Single topic, all users
		// - Multiple topics, all users
		// - All topics, all users.
		case message := <-server.broadcastChan:
			go broadcastMessageToTopic(ctx, server, message)

		// [CASE] Slow down the loop a bit.
		case <-ticker.C:
			// Only loop as often as the ticker's period.
		}

	}
}

// actuateRegisterClient is what the Server uses to actually register a Client.
// NOTE: This may be called on a goroutine, as it acts in reaction to data sent into a channel.
// DO NOT USE THIS METHOD OUTSIDE THIS FILE, PLEASE.
func actuateRegisterClient(ctx context.Context, server *Server, c *Client) {
	logrus.WithFields(logrus.Fields{
		"client.UserID":    c.UserID,
		"client.SessionID": c.SessionID(),
	}).Infof("actuateRegisterClient")

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	readyChan := make(chan bool)
	go c.HandleOutboundMessages(ctx, server, readyChan)

	// Track the client in various maps so that we can relate it to many topics.
	server.mutex.Lock()
	if currentClient, ok := server.clientsByUserID[c.UserID]; ok {
		if currentClient.SessionID() != c.SessionID() {
			currentClient.Close(errAlreadyRegistered)
		}
	}
	server.clientsByUserID[c.UserID] = c
	for _, topic := range c.Topics {
		userIDsForTopic, topicIsRegistered := server.userIDsByTopicDescriptor[topic]
		if !topicIsRegistered || userIDsForTopic == nil {
			server.userIDsByTopicDescriptor[topic] = make(map[string]struct{})
		}
		server.userIDsByTopicDescriptor[topic][c.UserID] = struct{}{}
	}
	server.mutex.Unlock()

	// Block and wait until the goroutine above is ready.
	// Users must user the sessionID in order to send a message, on top of any auth requirements.
	// We judiciously ue the goroutines below, since are already on a goroutine!
	<-readyChan
	go broadcastMessageToTopic(ctx, server, MessageForBroadcast{
		ToUserID: c.UserID,
		Message: Message{
			SessionID: c.SessionID(),
			Topic: TopicDescriptor{
				Type: TopicTypeAuth,
			},
		},
	})
	for _, topic := range c.Topics {
		fmt.Println("NotifyTopicOfJoin", time.Now().Format("2006-01-02 15:04:05 Z"), c.UserID, topic)
		go broadcastMessageToTopic(ctx, server, MessageForBroadcast{
			Message: Message{
				FromUserID: SystemUserID,
				Topic:      topic,
				Text:       fmt.Sprintf("UserID '%s' has joined Room '%s'.", c.UserID, topic.ChatRoomName),
			},
		})
	}
}

// actuateRegisterClient is what the Server uses to actually de-register a Client, for whatever reason.
// NOTE: This may be called on a goroutine, as it acts in reaction to data sent into a channel.
// DO NOT USE THIS METHOD OUTSIDE THIS FILE, PLEASE.
func actuateUnregisterClient(server *Server, c *Client, err error) {
	logrus.WithFields(logrus.Fields{
		"c.UserID": c.UserID,
	}).Infof("UnregisterClient")

	// Unregister Client from all Topics, then remove Client.
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if _, ok := server.clientsByUserID[c.UserID]; ok {
		delete(server.clientsByUserID, c.UserID)
		close(c.send)
	}
	for topic := range server.userIDsByTopicDescriptor {
		delete(server.userIDsByTopicDescriptor[topic], c.UserID)
	}
	c.Close(err)
}

// broadcastMessageToTopic is what the Server uses to trigger Clients to send messages to their connection(s).
// NOTE: This may be called on a goroutine, as it acts in reaction to data sent into a channel.
// DO NOT USE THIS METHOD OUTSIDE THIS FILE, PLEASE.
func broadcastMessageToTopic(ctx context.Context, server *Server, m MessageForBroadcast) {
	logEntry := logrus.WithFields(logrus.Fields{
		"message": m,
	})
	logEntry.Infof("broadcastMessageToTopic")

	// The `actuateSendMessageToClient` function is used by a couple cases below.
	actuateSendMessageToClient := func(ctx context.Context, server *Server, client *Client, broadcastMessage MessageForBroadcast) {
		select {

		// [CASE] Context was cancelled, and we need to bail!
		case <-ctx.Done():
			logrus.Infof("server context cancelled; quitting broadcastMessageToTopic goroutine")
			server.Close()
			return

		// [CASE] Send the message!
		case client.send <- broadcastMessage.Message:
			logrus.Infof("broadcastMessageToTopic: send")

		// [CASE] Unresponsive client.
		default:
			logrus.Infof("broadcastMessageToTopic: unregister due to unresponsiveness")
			actuateUnregisterClient(server, client, errUndeliverableClient)
		}
	}

	// Send to everyone?
	if m.Topic.Type == TopicTypeSystem && len(m.ToUserID) == 0 {
		logEntry.Infof("system message (start)")

		server.mutex.Lock()
		currentClientsByUserID := server.clientsByUserID
		server.mutex.Unlock()

		for _, client := range currentClientsByUserID {
			actuateSendMessageToClient(ctx, server, client, m)
		}
		logEntry.Infof("system message (complete)")
		return
	}

	// Send to individual User?
	if len(m.ToUserID) > 0 {

		server.mutex.Lock()
		client, clientIsRegistered := server.clientsByUserID[m.ToUserID]
		server.mutex.Unlock()

		if clientIsRegistered && client != nil {
			actuateSendMessageToClient(ctx, server, client, m)
		}
		logrus.WithFields(logrus.Fields{
			"message": m,
		}).Infof("broadcastMessageToTopic: completed (individual client)")
		return
	}

	// Send to Topic?
	server.mutex.Lock()
	currentUserIDsForTopic := server.userIDsByTopicDescriptor[m.Topic]
	server.mutex.Unlock()
	for userID := range currentUserIDsForTopic {

		server.mutex.Lock()
		client, clientIsRegistered := server.clientsByUserID[userID]
		server.mutex.Unlock()

		if clientIsRegistered && client != nil {
			actuateSendMessageToClient(ctx, server, client, m)
		}
	}
	logrus.WithFields(logrus.Fields{
		"message": m,
	}).Infof("broadcastMessageToTopic: completed (topical client)")
}
