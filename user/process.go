package user

import (
	"time"

	"github.com/DanielKrawisz/bmagent/powmgr"
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
)

// sendPow takes a message in wire format with all required information
// to send it over the network other than having proof-of-work run on it.
// It generates the correct parameters for running the proof-of-work and
// sends it to the proof-of-work queue with a function provided by the
// user that says what to do with the completed message when the
// proof-of-work is done.
func sendPow(pm *powmgr.Pow) func(object obj.Object, powData *pow.Data, done func([]byte)) {
	return func(object obj.Object, powData *pow.Data, done func([]byte)) {
		encoded := wire.Encode(object)
		q := encoded[8:] // exclude the nonce

		target := pow.CalculateTarget(uint64(len(q)),
			uint64(object.Header().Expiration().Sub(time.Now()).Seconds()), *powData)

		// Attempt to run pow on the message.
		pm.Run(target, q, func(n pow.Nonce) {
			// Put the nonce bytes into the encoded form of the message.
			q = append(n.Bytes(), q...)
			done(q)
		})
	}
}

// process takes a Bitmessage and does whatever needs to be done to it
// next in order to send it into the network. There are some steps in this
// process that take a bit of time, so they can't all be handled sequentially
// by one function. If we don't have the recipient's private key yet, we
// can't send the message and we have to send a pubkey request to him instead.
// On the other hand, messages might come in for which we already have the
// public key and then we can go on to the next step right away.
//
// The next step is proof-of-work, and that also takes time.
func (u *User) process(bmsg *email.Bmail) error {
	email.SMTPLog.Debug("process called.")
	outbox := u.boxes[OutboxFolderName]

	// sendPreparedMessage is used after we have looked up private keys and
	// generated an ack message, if applicable.
	sendPreparedMessage := func(object obj.Object, powData *pow.Data) error {
		email.SMTPLog.Debug("Generating pow for message.")
		err := outbox.saveBitmessage(bmsg)
		if err != nil {
			return err
		}

		// Put the prepared object in the pow queue and send it off
		// through the network.
		sendPow(u.pm)(object, powData, func(completed []byte) {
			err := func() error {
				// Save Bitmessage in outbox folder.
				err := u.boxes[OutboxFolderName].saveBitmessage(bmsg)
				if err != nil {
					return err
				}

				email.SMTPLog.Infof("Created bitmessage with hash %s", obj.InventoryHash(object).String())

				u.server.Send(completed)

				// Select new box for the message.
				var newBoxName string
				if bmsg.State.AckExpected {
					newBoxName = LimboFolderName
				} else {
					newBoxName = SentFolderName
				}

				bmsg.State.SendTries++
				bmsg.State.LastSend = time.Now()

				return u.Move(bmsg, OutboxFolderName, newBoxName)
			}()
			// We can't return the error any further because this function
			// isn't even run until long after process completes!
			if err != nil {
				email.SMTPLog.Error("process could not send message: ", err.Error())
			}
		})

		return nil
	}

	// First we attempt to generate the wire.Object form of the message.
	// If we can't, then it is possible that we don't have the recipient's
	// pubkey. That is not an error state. If no object and no error is
	// returned, this is what has happened. If there is no object, then we
	// can't proceed futher so process completes.
	object, data, err := u.generateObject(bmsg, outbox)

	if err != nil {
		if err == email.ErrGetPubKeySent {
			email.SMTPLog.Debug("process: Pubkey request sent.")

			// Try to save the message, as its state has changed.
			err := outbox.saveBitmessage(bmsg)
			if err != nil {
				return err
			}

			return nil
		} else if err == email.ErrAckMissing {
			email.SMTPLog.Debug("process: Generating ack.")
			ack, powData, err := u.generateAck(bmsg)
			if err != nil {
				email.SMTPLog.Debugf("process: could not generate ack: %v", err)
				return err
			}

			h := obj.InventoryHash(ack)

			// Save the ack.
			u.acks[*h] = bmsg.ImapData.UID

			sendPow(u.pm)(ack, powData, func(completed []byte) {
				err := func() error {
					// Add the ack to the message.
					bmsg.Ack = completed

					email.SMTPLog.Infof("Created ack message with hash %s", h.String())

					// Attempt to generate object again. This time it
					// should work so we return every error.
					object, objData, err := u.generateObject(bmsg, outbox)
					if err != nil {
						return err
					}

					return sendPreparedMessage(object, objData.Pow)
				}()
				// Once again, we can't return the error any further because
				// trySend is over by the time this function is run.
				if err != nil {
					email.SMTPLog.Error("process: could not send pow ", err.Error())
				}
			})

			return nil
		} else {
			email.SMTPLog.Debug("process: could not generate message.", err.Error())
			return err
		}
	}

	// If the object was generated successufully, do POW and send it.
	return sendPreparedMessage(object, data.Pow)
}
