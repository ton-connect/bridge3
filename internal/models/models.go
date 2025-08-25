package models

type SseMessage struct {
	EventId int64
	Message []byte
}

type BridgeMessage struct {
	From                string `json:"from"`
	Message             string `json:"message"`
	BridgeRequestSource string `json:"request_source"`
	TraceID             string `json:"trace_id"`
}

type BridgeRequestSource struct {
	Origin    string `json:"origin"`
	IP        string `json:"ip"`
	Time      string `json:"time"`
	ClientID  string `json:"client_id"`
	UserAgent string `json:"user_agent"`
}
