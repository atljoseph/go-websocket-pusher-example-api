package websocket

// TopicDescriptor holds all possible fields by which a TopicDescriptor could be described.
type TopicDescriptor struct {
	Type TopicType `json:"type,omitempty"`

	// ChatRoomName for the Topic described by TopicDescriptor.
	ChatRoomName string `json:"chat_room_name,omitempty"`
}

type TopicType string

const (
	TopicTypeSystem = TopicType("system")
	TopicTypeAuth   = TopicType("auth")
	TopicTypeChat   = TopicType("chat")
)
