bmagent
========

[![Build Status](https://travis-ci.org/monetas/bmclient.png?branch=master)]
(https://travis-ci.org/monetas/bmclient)

bmagent is a daemon handling Bitmessage identities for a single user. It acts
as an RPC client to bmd and an IMAP and SMTP server for interfacing with
traditional e-mail clients like Thunderbird, Outlook and Mail. It extensively
shares code and design philosophy with [btcwallet](https://github.com/btcsuite/btcwallet).

Private keys can be generated both randomly and deterministically using a seed.
Due to the sensitive nature of these private keys, they are stored encrypted
on disk using a key derived from the passphrase.

bmagent is not a network node and requires connection to a running instance of
bmd using JSON-RPC over websockets. Full bmd installation instructions can be
found [here](https://github.com/monetas/bmd).

No graphical frontends are planned as of right now. All communication between
the user and bmagent can be done using IMAP/SMTP or JSON-RPC over websockets
for the more advanced users or use cases.

## Installation

### Linux/BSD/POSIX - Build from Source

- Install Go according to the installation instructions here:
  http://golang.org/doc/install

- Run the following commands to obtain and install bm and all
  dependencies:
```bash
$ go get -u -v github.com/monetas/bmd/...
$ go get -u -v github.com/monetas/bmagent/...
```

- bmd and bmagent will now be installed in either ```$GOROOT/bin``` or
  ```$GOPATH/bin``` depending on your configuration. If you did not already
  add to your system path during the installation, we recommend you do so now.

## Updating

### Linux/BSD/POSIX - Build from Source

- Run the following commands to update bmagent, all dependencies, and install it:

```bash
$ go get -u -v github.com/monetas/bmd/...
$ go get -u -v github.com/monetas/bmagent/...
```

## Getting Started

The follow instructions detail how to get started with bmagent connecting to a
localhost bmd.

### Linux/BSD/POSIX/Source

- Run the following command to start bmd:

```bash
$ bmd -u rpcuser -P rpcpass
```

- Run the following command to create your first identity and send its public
  key to the network:

```bash
$ bmagent -u rpcuser -P rpcpass --create
```

- Run the following command to start bmagent:

```bash
$ bmagent -u rpcuser -P rpcpass
```

- Start your e-mail client and connect to localhost:143 for IMAP (with TLS) and
  localhost:587 for SMTP (with TLS) with rpcuser as username and rpcpass as
  password.

If everything appears to be working, it is recommended at this point to copy the
sample bmd and bmagent configurations and update with your RPC and IMAP/SMTP
username and password.

```bash
$ cp $GOPATH/src/github.com/monetas/bmd/sample-bmd.conf ~/.bmd/bmd.conf
$ cp $GOPATH/src/github.com/monetas/bmd/sample-bmclient.conf ~/.bmclient/bmclient.conf
$ $EDITOR ~/.bmd/bmd.conf
$ $EDITOR ~/.bmclient/bmclient.conf
```

## Issue Tracker

The [integrated github issue tracker](https://github.com/DanielKrawisz/bmagent/issues)
is used for this project.

## License

bmagent is licensed under the liberal ISC License.

## TODO

bmagent should only take what it needs from each object to determine whether it can
be decrypted. Get the whole object if it can be decrypted. 

There needs to be an implementation of email.IMAPMailbox for draft messages. 

rpc interface. 

Allow multiple users.
