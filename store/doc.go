// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package store is a persistent database for everything that bmclient needs
// for operation. This includes storing mailboxes and messages, proof of work
// queue, list of requested pubkeys.
//
// Take note that this store does not encrypt everything. Only messages contents
// are encrypted. Encryption scheme used is SalsaX20 stream cipher with Poly1305
// MAC, based on secretbox in NaCl.
//
// WARNING: If both your database and password were compromised, changing your
//          password won't accomplish anything. This is because store encrypts
//          the master key using the password you specify when the database
//          is created. A malicious user that knew your password and had a
//          previous copy of the store knows the master key and can decrypt all
//          messages.
//
// WARNING 2: Master key is kept in memory. The package is vulnerable to memory
//            reading malware.
package store
