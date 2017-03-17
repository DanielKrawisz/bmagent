package rpc

//go:generate protoc --go_out=plugins=grpc:. rpc.proto

import (
	"bytes"
	"fmt"
)

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

func (bi *BitmessageIdentity) Message() string {
	var address, label, behavior, poo, last string
	if bi.Address != nil {
		address = *bi.Address
		last = ", "
	}
	if bi.Label != nil {
		label = fmt.Sprintf("%s\"%s\"", last, *bi.Label)
		last = ", "
	}
	if bi.Behavior != nil {
		behavior = fmt.Sprintf("%sbehavior:%d", last, *bi.Behavior)
		last = ", "
	}
	if bi.Noncetrialsperbyte != nil && bi.Extrabytes != nil {
		poo = fmt.Sprintf("%spow:{%d, %d}", last, *bi.Noncetrialsperbyte, *bi.Extrabytes)
	}
	return fmt.Sprintf("%s%s%s%s", address, label, behavior, poo)

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

	return r.Address.String()
}

func (r *ListAddressesReply) Message() string {
	if r == nil {
		return ""
	}

	var b bytes.Buffer
	for i := 0; i < len(r.Addresses); i++ {
		if i != 0 {
			b.Write([]byte("\n"))
		}
		b.Write([]byte(r.Addresses[i].Message()))
	}

	return b.String()
}

func (r *HelpReply) Message() string {
	if r == nil {
		return ""
	}

	var b bytes.Buffer
	for i := 0; i < len(r.Instructions); i++ {
		if i != 0 {
			b.Write([]byte("\n"))
		}
		b.Write([]byte(r.Instructions[i]))
	}

	return b.String()
}
