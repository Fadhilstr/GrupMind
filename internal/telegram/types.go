package telegram

import "encoding/json"

// Update merepresentasikan Telegram Update object
type Update struct {
	UpdateID          int64          `json:"update_id"`
	Message           *Message       `json:"message,omitempty"`
	EditedMessage     *Message       `json:"edited_message,omitempty"`
	ChannelPost       *Message       `json:"channel_post,omitempty"`
	EditedChannelPost *Message       `json:"edited_channel_post,omitempty"`
	MyChatMember      *ChatMemberObj `json:"my_chat_member,omitempty"`
}

// User merepresentasikan pengguna atau bot Telegram
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// Chat merepresentasikan chat Telegram (pribadi, grup, supergroup, channel)
type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"` // "private", "group", "supergroup", "channel"
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// Message merepresentasikan pesan Telegram
type Message struct {
	MessageID  int64    `json:"message_id"`
	From       *User    `json:"from,omitempty"`
	SenderChat *Chat    `json:"sender_chat,omitempty"`
	Date       int64    `json:"date"`
	Chat       Chat     `json:"chat"`
	Text       string   `json:"text,omitempty"`
	ReplyTo    *Message `json:"reply_to_message,omitempty"`
}

// SendMessageRequest adalah payload untuk pemanggilan API sendMessage
type SendMessageRequest struct {
	ChatID                int64  `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview,omitempty"`
}

// APIResponse adalah wrapper respons standar dari Telegram Bot API
type APIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

// ChatMemberObj merepresentasikan update status keanggotaan bot dalam chat
type ChatMemberObj struct {
	Chat          Chat             `json:"chat"`
	From          User             `json:"from"`
	Date          int64            `json:"date"`
	OldChatMember ChatMemberStatus `json:"old_chat_member"`
	NewChatMember ChatMemberStatus `json:"new_chat_member"`
}

// ChatMemberStatus merepresentasikan status member dalam grup
type ChatMemberStatus struct {
	Status      string `json:"status"` // "creator", "administrator", "member", "restricted", "left", "kicked"
	CanSendMsgs bool   `json:"can_send_messages,omitempty"`
}
