package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"go-websocket-pusher-example-api/websocket"

	"github.com/sirupsen/logrus"
)

// InitializeWebsocketHandler handles websocket requests from the peer.
func InitializeWebsocketHandler(wsServer *websocket.Server, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.FormValue("user_id")
	if len(userID) == 0 {
		http.Error(w, "UserID not provided", http.StatusMethodNotAllowed)
	}

	chatRoomTopics := r.Form["rooms"]
	if len(chatRoomTopics) == 0 {
		http.Error(w, "Topics not provided", http.StatusMethodNotAllowed)
	}

	conn, err := websocket.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	topicDescriptors := []websocket.TopicDescriptor{}
	for _, chatRoomTopic := range chatRoomTopics {
		topicDescriptors = append(topicDescriptors, websocket.TopicDescriptor{
			ChatRoomName: chatRoomTopic,
			Type:         websocket.TopicTypeChat,
		})
	}
	client := websocket.NewClient(userID, topicDescriptors, conn, make(chan websocket.Message, 256))
	wsServer.RegisterClient(client)
}

type SendInboundWebSocketUserMessageOnTopic struct {
	SessionID string                    `json:"session_id"`
	UserID    string                    `json:"user_id"`
	Topic     websocket.TopicDescriptor `json:"topic"`
	Message   string                    `json:"message"`
}

// SendChatMessageHandler handles sending an inbound message to the API & websocket.
func SendChatMessageHandler(wsServer *websocket.Server, w http.ResponseWriter, r *http.Request) {
	logrus.WithFields(logrus.Fields{
		"r.URL": r.URL,
	}).Infof("SendChatMessageHandler")

	// Try to decode the request body into the struct. If there is an error,
	// respond to the client with the error message and a 400 status code.
	httpMessage := SendInboundWebSocketUserMessageOnTopic{}
	err := json.NewDecoder(r.Body).Decode(&httpMessage)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"httpMessage.UserID": httpMessage.UserID,
		}).Errorf("SendChatMessageHandler:Error")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logrus.WithFields(logrus.Fields{
		"r.URL":       r.URL,
		"httpMessage": httpMessage,
	}).Infof("SendChatMessageHandler:JSON")

	// Build the Websocket message, and force the type to be Chat.
	wsMessage := websocket.Message{
		FromUserID: httpMessage.UserID,
		Topic:      httpMessage.Topic,
		Text:       httpMessage.Message,
	}
	wsMessage.Topic.Type = websocket.TopicTypeChat
	messageToBroadcast := websocket.MessageForBroadcast{
		FromSessionID: httpMessage.SessionID,
		Message:       wsMessage,
	}
	logrus.WithFields(logrus.Fields{
		"messageToBroadcast.FromUserID": messageToBroadcast.FromUserID,
		"wsMessage":                     wsMessage,
	}).Infof("SendChatMessageHandler:Broadcast")
	if err := wsServer.BroadcastMessage(messageToBroadcast); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"messageToBroadcast.FromUserID": messageToBroadcast.FromUserID,
			"wsMessage":                     wsMessage,
		}).Errorf("SendChatMessageHandler:Broadcast")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// SendSystemMessageHandler handles sending an inbound message to the API & websocket.
func SendSystemMessageHandler(wsServer *websocket.Server, w http.ResponseWriter, r *http.Request) {
	logrus.WithFields(logrus.Fields{
		"r.URL": r.URL,
	}).Infof("SendSystemMessageHandler")

	// Try to decode the request body into the struct. If there is an error,
	// respond to the client with the error message and a 400 status code.
	httpMessage := SendInboundWebSocketUserMessageOnTopic{}
	err := json.NewDecoder(r.Body).Decode(&httpMessage)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"httpMessage.UserID": httpMessage.UserID,
		}).Errorf("SendSystemMessageHandler:Error")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logrus.WithFields(logrus.Fields{
		"r.URL":       r.URL,
		"httpMessage": httpMessage,
	}).Infof("SendSystemMessageHandler:JSON")

	// Build the Websocket message, and force the type to be Chat.
	wsMessage := websocket.Message{
		FromUserID: httpMessage.UserID,
		Topic: websocket.TopicDescriptor{
			Type: websocket.TopicTypeSystem,
		},
		Text: httpMessage.Message,
	}
	messageToBroadcast := websocket.MessageForBroadcast{
		// FromUserID:    websocket.SystemUserID,
		FromSessionID: httpMessage.SessionID,
		Message:       wsMessage,
	}
	logrus.WithFields(logrus.Fields{
		"messageToBroadcast.FromUserID": messageToBroadcast.FromUserID,
		"wsMessage":                     wsMessage,
	}).Infof("SendSystemMessageHandler:Broadcast")
	if err := wsServer.BroadcastMessage(messageToBroadcast); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"messageToBroadcast.FromUserID": messageToBroadcast.FromUserID,
			"wsMessage":                     wsMessage,
		}).Errorf("SendSystemMessageHandler:Broadcast")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}
