// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package idmgr is a key manager designed for use with bmagent, but which
// can also be used independently. It handles and handles hierarchically
// deterministic keys, imported keys (from PyBitmessage or other legacy
// systems), as well as channels.
//
// The file format for storage is a JSON document encrypted with SalsaX20 and
// MAC'd with Poly1305. The scheme used is byte compatible with secretbox from
// NaCl. Currently, it supports encrypting the entire file containing identity
// information (encryption and signing keys, required PoW constants, etc) and
// master key.
//
// Future support for separately encrypting master key and signing keys is
// planned. This could be useful for:
// * Read only bmclient nodes that are intended to only receive and decrypt
//   messages bound for them and are not likely to be used for sending messages.
// * Security conscious users who might be worried about memory-reading malware.
//   Note that even if we configure bmclient as such, the malware would still be
//   able to get the user's private encryption key. By the very nature of
//   Bitmessage, private encryption keys need to be in memory all the time.
//
// The contents of the encrypted file are arranged as such:
// Nonce (24 bytes) || Salt for PBKDF2 (32 bytes) || Encrypted data
//
// Both the nonce and salt are re-generated each time the file needs to be
// saved. This, along with a high number of PBKDF2 iterations (2^15), helps to
// ensure that an adversary with access to the key file will have an extremely
// hard time trying to bruteforce it. Rainbow tables will be useless.
//
// WARNING (again): The key manager is insecure against memory reading malware
//                  and is at the mercy of Go's garbage collector.
package idmgr
