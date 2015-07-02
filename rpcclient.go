// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec"
	"github.com/cenkalti/rpc2"
	"github.com/cenkalti/rpc2/jsonrpc"
	"github.com/gorilla/websocket"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/identity"
)

const (
	// Methods defined on RPC server.
	rpcAuth        = "Authenticate"
	rpcSendObject  = "SendObject"
	rpcGetIdentity = "GetIdentity"

	rpcSubscribeMessages    = "SubscribeMessages"
	rpcSubscribeBroadcasts  = "SubscribeBroadcasts"
	rpcSubscribeGetpubkeys  = "SubscribeGetpubkeys"
	rpcSubscribePubkeys     = "SubscribePubkeys"
	rpcSubscribeUnknownObjs = "SubscribeUnknownObjects"

	// Methods defined on RPC client.
	rpcClientHandleMessage    = "ReceiveMessage"
	rpcClientHandleBroadcast  = "ReceiveBroadcast"
	rpcClientHandleGetpubkey  = "ReceiveGetpubkey"
	rpcClientHandlePubkey     = "ReceivePubkey"
	rpcClientHandleUnknownObj = "ReceiveUnknownObject"
)

// rpcAuthArgs contains arguments for Authenticate.
type rpcAuthArgs struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// rpcClient is used to encapsulate an rpc2.Client struct and provide bmd
// specific features, along with
type rpcClient struct {
	*rpc2.Client

	quit    chan struct{}
	wg      sync.WaitGroup
	started bool
	quitMtx sync.Mutex
}

// newRPCClient creates a new RPC client connection. It takes a byte slice
// containing the trusted certificate of bmd and verifies the remote connection
// with it.
func newRPCClient(certs []byte) (*rpcClient, error) {
	var rpcLoc string
	var dialer *websocket.Dialer

	// Handle secure websocket connections.
	if !cfg.DisableClientTLS {
		rpcLoc = "wss://" + cfg.RPCConnect

		// Add certificate to trusted pool.
		roots := x509.NewCertPool()
		ok := roots.AppendCertsFromPEM(certs)
		if !ok {
			return nil, errors.New("Invalid TLS certificate specified")
		}

		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{
				RootCAs: roots,
			},
		}
	} else { // Insecure websocket connections.
		rpcLoc = "ws://" + cfg.RPCConnect
		dialer = websocket.DefaultDialer
	}

	ws, _, err := dialer.Dial(rpcLoc, nil)
	if err != nil {
		return nil, err
	}

	c := rpc2.NewClientWithCodec(jsonrpc.NewJSONCodec(ws.UnderlyingConn()))

	return &rpcClient{
		Client:  c,
		quit:    make(chan struct{}),
		started: false,
	}, nil
}

// rpcGetIDOut contains the output of GetIdentity.
type rpcGetIDOut struct {
	Address            string `json:"address"`
	NonceTrialsPerByte uint64 `json:"nonceTrialsPerByte"`
	ExtraBytes         uint64 `json:"extraBytes"`
	// base64 encoded bytes
	SigningKey    string `json:"signingKey"`
	EncryptionKey string `json:"encryptionKey"`
}

// GetIdentity returns the public identity corresponding to the given address
// if it exists.
func (c *rpcClient) GetIdentity(address string) (*identity.Public, error) {
	res := &rpcGetIDOut{}
	err := c.Client.Call(rpcGetIdentity, address, res)
	if err != nil {
		return nil, err
	}

	addr, err := bmutil.DecodeAddress(res.Address)
	if err != nil {
		return nil, err
	}
	skBytes, err := base64.StdEncoding.DecodeString(res.SigningKey)
	if err != nil {
		return nil, err
	}
	ekBytes, err := base64.StdEncoding.DecodeString(res.EncryptionKey)
	if err != nil {
		return nil, err
	}
	signKey, err := btcec.ParsePubKey(skBytes, btcec.S256())
	if err != nil {
		return nil, err
	}
	encKey, err := btcec.ParsePubKey(ekBytes, btcec.S256())
	if err != nil {
		return nil, err
	}

	return identity.NewPublic(signKey, encKey, res.NonceTrialsPerByte,
		res.ExtraBytes, addr.Version, addr.Stream), nil

}

// SendObject sends the given object to bmd so that it can send it out to the
// network.
func (c *rpcClient) SendObject(obj []byte) (uint64, error) {
	var counter uint64
	err := c.Call(rpcSendObject, base64.StdEncoding.EncodeToString(obj), &counter)
	if err != nil {
		return 0, err
	}
	return counter, nil
}

// Start starts the RPC client connection with bmd.
func (c *rpcClient) Start() error {
	go c.Client.Run()
	go c.disconnectHandler()

	var authOK bool
	err := c.Call(rpcAuth, &rpcAuthArgs{
		Username: cfg.BmdUsername,
		Password: cfg.BmdPassword,
	}, &authOK)
	if err != nil {
		return fmt.Errorf("Authentication failed: %v", err)
	}
	if !authOK {
		return errors.New("Authentication failed due to invalid credentials.")
	}

	c.quitMtx.Lock()
	c.started = true
	c.quitMtx.Unlock()
	c.wg.Add(1)

	return nil
}

// disconnectHandler takes care of any action to be performed if the websockets
// connection is disconnected.
func (c *rpcClient) disconnectHandler() {
	select {
	case <-c.Client.DisconnectNotify():

	}
}

// Stop disconnects the client and signals the shutdown of all goroutines
// started by Start.
func (c *rpcClient) Stop() {
	c.quitMtx.Lock()
	defer c.quitMtx.Unlock()

	select {
	case <-c.quit:
	default:
		close(c.quit)
		c.Client.Close()
	}
}

// WaitForShutdown blocks until both the client has finished disconnecting
// and all handlers have exited.
func (c *rpcClient) WaitForShutdown() {
	c.wg.Wait()
}
