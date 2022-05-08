package websocket

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

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
}

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

func (s *Server) TopicIsRegistered(topic TopicDescriptor) bool {
	_, ok := s.userIDsByTopicDescriptor[topic]
	return ok
}

func (s *Server) RegisterClient(client *Client) {
	logrus.WithFields(logrus.Fields{
		"client.UserID":    client.UserID,
		"client.SessionID": client.SessionID(),
	}).Infof("RegisterClient")
	// Send this client into the `registerClientChan`. The receiver will register the Client for the declared Topics.
	s.registerClientChan <- client
}

func (s *Server) UnregisterClient(client *Client) {
	s.unregisterClientChan <- client
}

func (s *Server) BroadcastMessage(bm MessageForBroadcast) error {

	if bm.Topic == emptyTopic {
		return fmt.Errorf("failed to broadcast message from '%s' with no topic", bm.FromUserID)
	}

	// If this message came from a user, validate the SessionID!
	if len(bm.FromSessionID) > 0 {

		// Confirm sending user has a client registered.
		s.mutex.Lock()
		fromClient, fromClientIsRegistered := s.clientsByUserID[bm.FromUserID]
		s.mutex.Unlock()
		if !fromClientIsRegistered || fromClient == nil {
			return fmt.Errorf("failed to broadcast message since user '%s' has no registered client", bm.FromUserID)
		}

		// Confirm message is from the correct sessionID.
		if bm.FromSessionID != fromClient.SessionID() {
			return fmt.Errorf("failed to broadcast message since it came from an invalid session")
		}
	}

	// Validate the server has this topic.
	if topicIsRegistered := s.TopicIsRegistered(bm.Topic); !topicIsRegistered {
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
		s.mutex.Lock()
		toClient, toClientIsRegistered := s.clientsByUserID[bm.FromUserID]
		s.mutex.Unlock()
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
		"len(s.broadcastChan)": len(s.broadcastChan),
	}).Infof("s.broadcastChan <- bm.Message")

	// Send this message to be broadcasted. The receiver will send it to everybody on the topic.
	s.broadcastChan <- bm
	return nil
}

// run should be ran on a goroutine.
func (s *Server) Run() {

	// Infinite loop.
	for {
		// Select the first channel that has a value in it.
		select {

		// [CASE] Should we Register this Client?
		// Unregister & Re-register Client.
		case client := <-s.registerClientChan:
			actuateRegisterClient(s, client)

		// [CASE] Should we Unregister this Client?
		case client := <-s.unregisterClientChan:
			actuateUnregisterClient(s, client)

		// [CASE] Do we have a message to send?
		// Send messages to potential targets:
		// - Single user
		// - Multiple users
		// - Single topic, all users
		// - Multiple topics, all users
		// - All topics, all users.
		case message := <-s.broadcastChan:
			go broadcastMessageToTopic(s, message)

			// default:
			// 	logrus.Infof("hey!")
		}
	}
}

func actuateRegisterClient(s *Server, c *Client) {
	logrus.WithFields(logrus.Fields{
		"client.UserID":    c.UserID,
		"client.SessionID": c.SessionID(),
	}).Infof("actuateRegisterClient")

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	readyChan := make(chan bool)
	go c.HandleOutboundMessages(s, readyChan)

	// Track the client in various maps so that we can relate it to many topics.
	s.mutex.Lock()
	if currentClient, ok := s.clientsByUserID[c.UserID]; ok {
		if currentClient.SessionID() != c.SessionID() {
			currentClient.Close(fmt.Errorf("UserID has been re-registered"))
		}
	}
	s.clientsByUserID[c.UserID] = c
	for _, topic := range c.Topics {
		userIDsForTopic, topicIsRegistered := s.userIDsByTopicDescriptor[topic]
		if !topicIsRegistered || userIDsForTopic == nil {
			s.userIDsByTopicDescriptor[topic] = make(map[string]struct{})
		}
		s.userIDsByTopicDescriptor[topic][c.UserID] = struct{}{}
	}
	s.mutex.Unlock()

	// Block and wait until the goroutine above is ready.
	// Users must user the sessionID in order to send a message, on top of any auth requirements.
	<-readyChan
	// err := c.SendSessionID()
	// if err != nil {
	// 	return
	// }
	// timer := time.NewTimer(time.Second * 1)
	// defer timer.Stop()
	// <-timer.C
	// logrus.WithFields(logrus.Fields{
	// 	"client.UserID":    c.UserID,
	// 	"client.SessionID": c.SessionID(),
	// }).Infof("notify client of sessionID")
	// if err := s.BroadcastMessage(MessageForBroadcast{
	// 	FromUserID: systemUserID,
	// 	ToUserID:   c.UserID,
	// 	Message: Message{
	// 		Type:      MessageTypeConfirmRegistrationEvent,
	// 		SessionID: c.SessionID(),
	// 		Topic:     emptyTopic,
	// 		// Text: NONE,
	// 	},
	// }); err != nil {
	// 	logrus.WithError(err).Errorf("failed to notify user of session ID")
	// }
	go broadcastMessageToTopic(s, MessageForBroadcast{
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
		go broadcastMessageToTopic(s, MessageForBroadcast{
			Message: Message{
				FromUserID: SystemUserID,
				Topic:      topic,
				Text:       fmt.Sprintf("UserID '%s' has joined Room '%s'.", c.UserID, topic.ChatRoomName),
			},
		})
	}
}

func actuateUnregisterClient(s *Server, c *Client) {
	logrus.WithFields(logrus.Fields{
		"c.UserID": c.UserID,
	}).Infof("UnregisterClient")

	// Unregister Client from all Topics, then remove Client.
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.clientsByUserID[c.UserID]; ok {
		delete(s.clientsByUserID, c.UserID)
		close(c.send)
	}
	for topic := range s.userIDsByTopicDescriptor {
		delete(s.userIDsByTopicDescriptor[topic], c.UserID)
	}
}

func broadcastMessageToTopic(s *Server, m MessageForBroadcast) {
	logrus.WithFields(logrus.Fields{
		"message": m,
	}).Infof("broadcastMessageToTopic")

	// Send to everyone?
	if m.Topic.Type == TopicTypeSystem {

		s.mutex.Lock()
		currentClientsByUserID := s.clientsByUserID
		s.mutex.Unlock()

		for userID, client := range currentClientsByUserID {
			select {
			case client.send <- m.Message:
				logrus.WithFields(logrus.Fields{}).Infof("broadcastMessageToTopic: send (system)")
			default:
				logrus.WithFields(logrus.Fields{}).Infof("broadcastMessageToTopic: default (system)")
				close(client.send)
				s.mutex.Lock()
				delete(s.clientsByUserID, userID)
				s.mutex.Unlock()
			}
		}
	}

	// Send to individual User?
	if len(m.ToUserID) > 0 {

		s.mutex.Lock()
		client, clientIsRegistered := s.clientsByUserID[m.ToUserID]
		s.mutex.Unlock()

		if clientIsRegistered && client != nil {
			client.send <- m.Message
			logrus.WithFields(logrus.Fields{}).Infof("broadcastMessageToTopic: send (individual client)")
			return
		}
	}

	// Send to Topic?

	// Pivot; Send message to all interested clients (except the FromUserID).
	s.mutex.Lock()
	currentUserIDsForTopic := s.userIDsByTopicDescriptor[m.Topic]
	s.mutex.Unlock()
	for userID := range currentUserIDsForTopic {

		s.mutex.Lock()
		client, clientIsRegistered := s.clientsByUserID[userID]
		s.mutex.Unlock()

		if clientIsRegistered && client != nil {
			select {
			case client.send <- m.Message:
				logrus.WithFields(logrus.Fields{}).Infof("broadcastMessageToTopic: send (topical client)")
			default:
				logrus.WithFields(logrus.Fields{}).Infof("broadcastMessageToTopic: default (topical client)")
				close(client.send)
				s.mutex.Lock()
				delete(s.clientsByUserID, userID)
				s.mutex.Unlock()
			}
		}
	}

	logrus.WithFields(logrus.Fields{
		"message": m,
	}).Infof("broadcastMessageToTopic: completed")
}
