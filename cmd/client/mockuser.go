package main

import "github.com/DanielKrawisz/bmutil"

type mockUser struct {
}

func (mu *mockUser) NewAddress(tag string, sendAck bool) bmutil.Address {
	return nil
}
