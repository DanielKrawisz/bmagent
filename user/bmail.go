package user

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"time"

	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
)

// GenerateAck creates an Ack message
func GenerateAck(s email.ServerOps, m *email.Bmail) (ack obj.Object, powData *pow.Data, err error) {
	// If this is a broadcast message, no ack is expected.
	if m.To == "broadcast@bm.agent" {
		return nil, nil, errors.New("No acks on broadcast messages.")
	}

	// Get our private key.
	fromAddr, err := email.ToBM(m.From)
	if err != nil {
		return nil, nil, err
	}
	from := s.GetPrivateID(fromAddr)

	addr := from.ToPublic().Address

	// Add the message, which is a random number.
	buf := new(bytes.Buffer)
	num := rand.Int31()
	err = binary.Write(buf, binary.LittleEndian, num)
	if err != nil {
		return nil, nil, err
	}

	// We don't save the message because it still needs POW done on it.
	return wire.NewMsgObject(
		wire.NewObjectHeader(0,
			time.Now().Add(s.GetObjectExpiry(wire.ObjectTypeMsg)),
			wire.ObjectTypeMsg,
			obj.MessageVersion,
			addr.Stream,
		), buf.Bytes()), &from.Data, nil
}
