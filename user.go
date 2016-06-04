// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	
	"github.com/DanielKrawisz/bmagent/keymgr"
)

// User contains the information relevant from the
// standpoint of the server that is associated with the individual user. 
type User struct {
	Keys     *keymgr.Manager
	Path     string
	Username string
	Pass     []byte
}

func (u *User) SaveKeyfile() {	
	saveKeyfile(u.Keys, u.Path, u.Pass)
}

func saveKeyfile(keys *keymgr.Manager, path string, pass []byte) {
	var serialized []byte
	var err error
	
	if pass == nil {		
		serialized, err = keys.ExportPlaintext()
	} else {	
		serialized, err = keys.ExportEncrypted(pass)
	}
	
	if err != nil {
		log.Criticalf("Failed to serialize key file: %v", err)
		return
	}

	err = ioutil.WriteFile(path, serialized, 0600)
	if err != nil {
		log.Criticalf("Failed to write key file: %v", err)
	}
}