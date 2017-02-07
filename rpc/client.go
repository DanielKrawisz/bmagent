// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"errors"
	"fmt"
	"sync"
	"time"

	pb "github.com/DanielKrawisz/bmd/rpcproto"
	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/btcsuite/btcd/btcec"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
)

var (
	// ErrIdentityNotFound is returned by GetIdentity.
	ErrIdentityNotFound = errors.New("identity not found")
)

// ClientConfig are configuration options for the RPC client to bmd.
type ClientConfig struct {
	// DisableTLS specifies whether TLS should be disabled for a connection to
	// bmd.
	DisableTLS bool

	// CAFile is  the file containing root certificates to authenticate a TLS
	// connection with bmd.
	CAFile string

	// ConnectTo is the hostname/IP and port of bmd RPC server to connect to.
	ConnectTo string

	// Username is the username to use for authentication with bmd.
	Username string

	// Password is the password to use for authentication with bmd.
	Password string

	// The timeout duration.
	Timeout time.Duration
}

// Client encapsulates a connection to bmd and provides helper methods for
// retrieving relevant data.
type Client struct {
	bmd           pb.BmdClient
	conn          *grpc.ClientConn
	msgFunc       func(counter uint64, msg []byte)
	broadcastFunc func(counter uint64, msg []byte)
	getpubkeyFunc func(counter uint64, msg []byte)
	quit          chan struct{}
	wg            sync.WaitGroup
	started       bool
	shutdown      bool
	quitMtx       sync.Mutex
}

// NewClient creates a new RPC connection to bmd.
func NewClient(cfg *ClientConfig, msg, broadcast, getpubkey func(counter uint64, msg []byte)) (*Client, error) {

	opts := []grpc.DialOption{
		grpc.WithPerRPCCredentials(
			pb.NewBasicAuthCredentials(cfg.Username, cfg.Password)),
		grpc.WithTimeout(cfg.Timeout)}

	if cfg.DisableTLS {
		opts = append(opts, grpc.WithInsecure())
	} else {
		creds, err := credentials.NewClientTLSFromFile(cfg.CAFile, "")
		if err != nil {
			return nil, fmt.Errorf("Failed to create TLS credentials %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	conn, err := grpc.Dial(cfg.ConnectTo, opts...)

	if err != nil {
		return nil, fmt.Errorf("Failed to dial: %v", err)
	}
	bmd := pb.NewBmdClient(conn)

	// Verify credentials.
	_, err = bmd.GetIdentity(context.Background(), &pb.GetIdentityRequest{
		Address: "InvalidAddress",
	})
	code := grpc.Code(err)
	if code == codes.Unauthenticated || code == codes.PermissionDenied {
		return nil, errors.New("authentication failure; invalid username/password")
	} else if code != codes.InvalidArgument {
		return nil, fmt.Errorf("Unexpected error verifying credentials: %v", err)
	}

	return &Client{
		bmd:           bmd,
		conn:          conn,
		quit:          make(chan struct{}),
		started:       false,
		msgFunc:       msg,
		broadcastFunc: broadcast,
		getpubkeyFunc: getpubkey,
	}, nil
}

// GetIdentity returns the public identity corresponding to the given address
// if it exists.
func (c *Client) GetIdentity(address string) (*identity.Public, error) {
	addr, err := bmutil.DecodeAddress(address)
	if err != nil {
		return nil, fmt.Errorf("Address decode failed: %v", addr)
	}

	res, err := c.bmd.GetIdentity(context.Background(), &pb.GetIdentityRequest{
		Address: address,
	})
	if grpc.Code(err) == codes.NotFound {
		return nil, ErrIdentityNotFound
	} else if err != nil {
		return nil, err
	}

	signKey, err := btcec.ParsePubKey(res.SigningKey, btcec.S256())
	if err != nil {
		return nil, err
	}
	encKey, err := btcec.ParsePubKey(res.EncryptionKey, btcec.S256())
	if err != nil {
		return nil, err
	}

	return identity.NewPublic(signKey, encKey,
		&pow.Data{
			res.NonceTrials,
			res.ExtraBytes,
		}, addr.Version, addr.Stream), nil
}

// SendObject sends the given object to bmd so that it can send it out to the
// network.
func (c *Client) SendObject(obj []byte) (uint64, error) {
	serverLog.Trace("Sending object into the network.")
	res, err := c.bmd.SendObject(context.Background(), &pb.Object{Contents: obj})
	if err != nil {
		return 0, err
	}
	return res.Counter, nil
}

// Start starts the RPC client connection with bmd.
func (c *Client) Start(msgCounter, broadcastCounter, getpubkeyCounter uint64) {
	c.quitMtx.Lock()
	c.started = true
	defer c.quitMtx.Unlock()

	// Start messages processor.
	c.wg.Add(1)
	go c.processObjects(pb.ObjectType_MESSAGE, msgCounter, c.msgFunc)

	// Start broadcast processor.
	c.wg.Add(1)
	go c.processObjects(pb.ObjectType_BROADCAST, broadcastCounter, c.broadcastFunc)

	// Start getpubkey processor.
	c.wg.Add(1)
	go c.processObjects(pb.ObjectType_GETPUBKEY, getpubkeyCounter, c.getpubkeyFunc)
}

// processObjects receives objects from bmd and runs the specified function for
// each object.
func (c *Client) processObjects(objType pb.ObjectType, fromCounter uint64,
	f func(counter uint64, msg []byte)) {

	defer c.wg.Done()

	stream, err := c.bmd.GetObjects(context.Background(), &pb.GetObjectsRequest{
		ObjectType:  objType,
		FromCounter: fromCounter,
	})
	if err != nil {
		clientLog.Errorf("Failed to call GetObjects for messages: %v", err)
		return
	}

	clientLog.Infof("Starting to receive %s objects from counter %d.", objType,
		fromCounter)
	for {
		select {
		case <-c.quit:
			return
		default:
			obj, err := stream.Recv()
			// We shouldn't show an error if the system is just shutting down.
			if err != nil {
				if err.Error() != "rpc error: code = 13 desc = transport is closing" {
					clientLog.Criticalf("Failed to receive object of type %s: %v", objType, err)
				}
				return
			}
			f(obj.Counter, obj.Contents)
		}
	}
}

// Stop disconnects the client and signals the shutdown of all goroutines
// started by Start.
func (c *Client) Stop() {
	c.quitMtx.Lock()
	defer c.quitMtx.Unlock()

	close(c.quit)
	c.conn.Close()

	// This may eliminate a possible memory leak, since the server struct
	// from which these functions arose also has a pointer to this rpc client.
	c.msgFunc = nil
	c.broadcastFunc = nil
	c.getpubkeyFunc = nil
}

// WaitForShutdown blocks until both the client has finished disconnecting
// and all handlers have exited.
func (c *Client) WaitForShutdown() {
	c.wg.Wait()
}
