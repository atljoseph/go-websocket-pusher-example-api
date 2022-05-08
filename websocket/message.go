package websocket

// type MessageType string

// const (
// 	MessageTypeSystemEvent              = MessageType("system")
// 	MessageTypeConfirmRegistrationEvent = MessageType("confirm-registration")
// 	MessageTypeChatEvent                = MessageType("chat")
// )

type Message struct {

	// SessionID is only set for Auth messages.
	SessionID string `json:"session_id,omitempty"`

	// FromUserID would ideally be just the a username.
	FromUserID string
	Topic      TopicDescriptor `json:"topic,omitempty"`
	// Type      MessageType     `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type MessageForBroadcast struct {

	// FromSessionID is specified when the originator is not the system.
	FromSessionID string

	// ToUserID is specified when a specific user is targeted.
	ToUserID string

	// Message is what gets sent over the websocket to the client who initiated the connection.
	Message
}
