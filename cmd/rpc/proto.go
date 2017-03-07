package rpc

//go:generate protoc --go_out=plugins=grpc:. rpc.proto

func Message(r *BMRPCReply) string {
	if r.Reply == nil {
		return ""
	}

	switch x := r.Reply.(type) {
	default:
		return "unrecognized message"
	case *BMRPCReply_ErrorReply:
		return x.ErrorReply.Message()
	case *BMRPCReply_Newaddress:
		return x.Newaddress.Message()
	case *BMRPCReply_Listaddresses:
		return x.Listaddresses.Message()
	case *BMRPCReply_HelpReply:
		return x.HelpReply.Message()
	}
}

func (r *ErrorReply) Message() string {
	if r == nil {
		return ""
	}

	return ""
}

func (r *NewAddressReply) Message() string {
	if r == nil {
		return ""
	}

	return ""
}

func (r *ListAddressesReply) Message() string {
	if r == nil {
		return ""
	}

	return ""
}

func (r *HelpReply) Message() string {
	if r == nil {
		return ""
	}

	return "You have been helped."
}
