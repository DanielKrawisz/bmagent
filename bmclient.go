// Originally derived from: btcsuite/btwallet/config.go
// Copyright (c) 2013-2014 The btcsuite developers

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
)

var (
	cfg             *config
	shutdownChannel = make(chan struct{})
)

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Work around defer not working after os.Exit.
	if err := bmclientMain(); err != nil {
		os.Exit(1)
	}
}

// bmclientMain is a work-around main function that is required since deferred
// functions (such as log flushing) are not called with calls to os.Exit.
// Instead, main runs this function and checks for a non-nil error, at which
// point any defers have already run, and if the error is non-nil, the program
// can be exited with an error exit status.
func bmclientMain() error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	tcfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	cfg = tcfg
	defer backendLog.Flush()

	if cfg.Profile != "" {
		go func() {
			listenAddr := net.JoinHostPort("", cfg.Profile)
			log.Infof("Profile server listening on %s", listenAddr)
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			log.Errorf("%v", http.ListenAndServe(listenAddr, nil))
		}()
	}
	/*
		// Load the identities and message databases. The identities database must
		// have been created with the --create option already or this will return an
		// appropriate error.
		keysDb, msgDb, err := openDatabases()
		if err != nil {
			log.Errorf("%v", err)
			return err
		}
		defer keysDb.Close()
		defer msgDb.Close()

		// Create and start the RPC server
		server, err := newRPCServer(cfg.SvrListeners, cfg.RPCMaxClients)
		if err != nil {
			log.Errorf("Unable to create HTTP server: %v", err)
			return err
		}
		server.Start()
		server.SetWallet(wallet)

		// Shutdown the server if an interrupt signal is received.
		addInterruptHandler(server.Stop)

		go func() {
			// Read CA certs and create the RPC client.
			var certs []byte
			if !cfg.DisableClientTLS {
				certs, err = ioutil.ReadFile(cfg.CAFile)
				if err != nil {
					log.Warnf("Cannot open CA file: %v", err)
					// If there's an error reading the CA file, continue
					// with nil certs and without the client connection
					certs = nil
				}
			} else {
				log.Info("Client TLS is disabled")
			}
		}()

		// Wait for the server to shutdown either due to a stop RPC request
		// or an interrupt.
		server.WaitForShutdown()
	*/

	// Monitor for graceful server shutdown and signal the main goroutine
	// when done. This is done in a separate goroutine rather than waiting
	// directly so the main goroutine can be signaled for shutdown by either
	// a graceful shutdown or from the main interrupt handler. This is
	// necessary since the main goroutine must be kept running long enough
	// for the interrupt handler goroutine to finish.
	go func() {
		//server.WaitForShutdown()
		//srvrLog.Infof("Server shutdown complete")
		shutdownChannel <- struct{}{}
	}()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-shutdownChannel
	log.Info("Shutdown complete")
	return nil
}
