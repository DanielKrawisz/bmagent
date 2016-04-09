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
	Pass     []byte
}

func (u *User) SaveKeyfile() {
	var serialized []byte
	var err error
	
	if u.Pass == nil {
		if !cfg.PlaintextDB {
			log.Warn("No password supplied for keyfile.")
		}
		
		serialized, err = u.Keys.ExportPlaintext()
	} else {	
		serialized, err = u.Keys.ExportEncrypted(u.Pass)
	}
	
	if err != nil {
		log.Criticalf("Failed to serialize key file: %v", err)
		return
	}

	err = ioutil.WriteFile(u.Path, serialized, 0600)
	if err != nil {
		log.Criticalf("Failed to write key file: %v", err)
	}
}