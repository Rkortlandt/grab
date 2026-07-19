package comm

import "encoding/gob"

type ClipboardRequest struct{}
type HistoryRequest struct{}
type StatusRequest struct{}
type SwitchHistoryRequest struct {
	Index int
}

type GrabRequest struct {
	Files []string
}

type ClipboardResponse struct {
	Files []string
	Error string
}

type HistoryResponse struct {
	History [][]string
	Error   string
}

type ActionResponse struct {
	Message string
	Error   string
}

func init() {

	gob.Register(ClipboardResponse{})
	gob.Register(HistoryResponse{})
	gob.Register(ActionResponse{})
}
