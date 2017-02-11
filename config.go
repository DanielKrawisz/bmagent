// Originally derived from: btcsuite/btcwallet/config.go
// Copyright (c) 2013-2014 The btcsuite developers

// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/btcsuite/btcutil"
	flags "github.com/jessevdk/go-flags"
)

const (
	defaultCAFilename       = "bmd.cert"
	defaultConfigFilename   = "bmagent.conf"
	defaultLogLevel         = "info"
	defaultLogDirname       = "logs"
	defaultLogFilename      = "bmagent.log"
	defaultDisallowFree     = false
	defaultRPCMaxClients    = 10
	defaultRPCMaxWebsockets = 25

	defaultBmdPort  = 8442
	defaultRPCPort  = 8446
	defaultIMAPPort = 1143
	defaultSMTPPort = 1587

	keyfileName = "keys.dat"
	storeDbName = "store.db"

	defaultPowHandler = "parallel"

	defaultMsgExpiry        = time.Hour * 60      // 2.5 days
	defaultBroadcastExpiry  = time.Hour * 48      // 2 days
	defaultPubkeyExpiry     = time.Hour * 24 * 14 // 14 days
	defaultGetpubkeyExpiry  = time.Hour * 24 * 14 // 14 days
	defaultUnknownObjExpiry = time.Hour * 24

	defaultLogConsole = true

	defaultGenKeys = -1
)

var (
	defaultDataDir     = btcutil.AppDataDir("bmagent", false)
	bmdDataDir         = btcutil.AppDataDir("bmd", false)
	bmdHomedirCAFile   = filepath.Join(bmdDataDir, "rpc.cert")
	defaultConfigFile  = filepath.Join(defaultDataDir, defaultConfigFilename)
	defaultTLSKeyFile  = filepath.Join(defaultDataDir, "tls.key")
	defaultTLSCertFile = filepath.Join(defaultDataDir, "tls.cert")
	defaultLogDir      = filepath.Join(defaultDataDir, defaultLogDirname)
)

// Config contains the configuration information read from the command line and
// from the config file.
type Config struct {
	ShowVersion bool   `short:"V" long:"version" description:"Display version information and exit"`
	DataDir     string `short:"D" long:"datadir" description:"Directory to store key file and the data store"`
	LogDir      string `long:"logdir" description:"Directory to log output"`

	DebugLevel string `short:"d" long:"debuglevel" description:"Logging level {trace, debug, info, warn, error, critical}"`
	ConfigFile string `short:"C" long:"configfile" description:"Path to configuration file"`

	Create        bool   `long:"create" description:"Create the identity and message databases if they don't exist"`
	ImportKeyFile string `long:"importkeyfile" description:"Path to keys.db from PyBitmessage. If set, private keys from this file are imported into bmagent"`
	NoPass        bool   `long:"nopass" description:"Keyfile and database are created unencrypted"`
	Seed          string `long:"seed" description:"Used with --create. Used to specify the seed for a new keyset"`

	EnableRPC     bool     `long:"rpc" description:"Enable built-in RPC server -- NOTE: The RPC server is disabled by default"`
	RPCListeners  []string `long:"rpclisten" description:"Listen for RPC/websocket connections on this interface/port (default port: 8446)"`
	IMAPListeners []string `long:"imaplisten" description:"Listen for IMAP connections on this interface/port (default port: 143)"`
	SMTPListeners []string `long:"smtplisten" description:"Listen for SMTP connections on this interface/port (default port: 587)"`

	TLSCert          string `long:"rpccert" description:"File containing the certificate file"`
	TLSKey           string `long:"rpckey" description:"File containing the certificate key"`
	DisableServerTLS bool   `long:"noservertls" description:"Disable TLS for the RPC, IMAP and SMTP servers -- NOTE: This is only allowed if the servers are all bound to localhost"`
	DisableClientTLS bool   `long:"noclienttls" description:"Disable TLS for the RPC client -- NOTE: This is only allowed if the RPC client is connecting to localhost"`
	CAFile           string `long:"cafile" description:"File containing root certificates to authenticate a TLS connection with bmd"`
	RPCConnect       string `short:"c" long:"rpcconnect" description:"Hostname/IP and port of bmd RPC server to connect to (default localhost:8442)"`

	Username    string `short:"u" long:"username" description:"Username for clients (RPC/IMAP/SMTP) and bmd authorization"`
	Password    string `short:"P" long:"password" default-mask:"-" description:"Password for clients (RPC/IMAP/SMTP) and bmd authorization"`
	BmdUsername string `long:"bmdusername" description:"Alternative username for bmd authorization"`
	BmdPassword string `long:"bmdpassword" default-mask:"-" description:"Alternative password for bmd authorization"`

	Profile string `long:"profile" description:"Enable HTTP profiling on given port -- NOTE port must be between 1024 and 65536"`

	ProofOfWork     string        `long:"pow" description:"Choose proof-of-work handler. Options: {sequential, parallel}"`
	PowThreads      int           `long:"powthreads" description:"Number of threads to use for parallel proof-of-work calculation. It should not be greater than the number of cores"`
	MsgExpiry       time.Duration `long:"msgexpiry" description:"Time after which a message sent out should expire, more means more time for POW calculations"`
	BroadcastExpiry time.Duration `long:"broadcastexpiry" description:"Time after which a broadcast sent out should expire, more means more time for POW calculations"`

	LogConsole bool `long:"logconsole" description:"display logs to console."`

	GenKeys int16 `long:"genkeys" description:"number of new keys to generate."`

	powHandler func(target uint64, hash []byte) pow.Nonce
	storePath  string

	// TODO there should not be a global path for a single key file.
	keyfilePath string
	keyfilePass []byte
}

// cleanAndExpandPath expands environement variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		homeDir := filepath.Dir(defaultDataDir)
		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but they variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	switch logLevel {
	case "trace":
		fallthrough
	case "debug":
		fallthrough
	case "info":
		fallthrough
	case "warn":
		fallthrough
	case "error":
		fallthrough
	case "critical":
		return true
	}
	return false
}

var localhostListeners = map[string]struct{}{
	"localhost": struct{}{},
	"127.0.0.1": struct{}{},
	"::1":       struct{}{},
}

// verifyListeners is used to verify if any non-localhost listen address is
// being used with no TLS.
func verifyListeners(addrs []string, service string, funcName string,
	usageMessage string) error {
	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			str := "%s: %s listen interface '%s' is invalid: %v"
			err := fmt.Errorf(str, funcName, service, addr, err)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return err
		}
		if _, ok := localhostListeners[host]; !ok {
			str := "%s: the --noservertls option may not be used when binding" +
				" %s to non localhost addresses: %s"
			err := fmt.Errorf(str, funcName, service, addr)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return err
		}
	}
	return nil
}

// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
func supportedSubsystems() []string {
	// Convert the subsystemLoggers map keys to a slice.
	subsystems := make([]string, 0, len(subsystemLoggers))
	for subsysID := range subsystemLoggers {
		subsystems = append(subsystems, subsysID)
	}

	// Sort the subsytems for stable display.
	sort.Strings(subsystems)
	return subsystems
}

// parseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
func parseAndSetDebugLevels(debugLevel string) error {
	// When the specified string doesn't have any delimters, treat it as
	// the log level for all subsystems.
	if !strings.Contains(debugLevel, ",") && !strings.Contains(debugLevel, "=") {
		// Validate debug log level.
		if !validLogLevel(debugLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, debugLevel)
		}

		// Change the logging level for all subsystems.
		setLogLevels(debugLevel)

		return nil
	}

	// Split the specified string into subsystem/level pairs while detecting
	// issues and update the log levels accordingly.
	for _, logLevelPair := range strings.Split(debugLevel, ",") {
		if !strings.Contains(logLevelPair, "=") {
			str := "The specified debug level contains an invalid " +
				"subsystem/level pair [%v]"
			return fmt.Errorf(str, logLevelPair)
		}

		// Extract the specified subsystem and log level.
		fields := strings.Split(logLevelPair, "=")
		subsysID, logLevel := fields[0], fields[1]

		// Validate subsystem.
		if _, exists := subsystemLoggers[subsysID]; !exists {
			str := "The specified subsystem [%v] is invalid -- " +
				"supported subsytems %v"
			return fmt.Errorf(str, subsysID, supportedSubsystems())
		}

		// Validate log level.
		if !validLogLevel(logLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, logLevel)
		}

		setLogLevel(subsysID, logLevel)
	}

	return nil
}

// removeDuplicateAddresses returns a new slice with all duplicate entries in
// addrs removed.
func removeDuplicateAddresses(addrs []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, val := range addrs {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = true
		}
	}
	return result
}

// normalizeAddress returns addr with the passed default port appended if
// there is not already a port specified.
func normalizeAddress(addr string, defaultPort int) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, strconv.Itoa(defaultPort))
	}
	return addr
}

// normalizeAddresses returns a new slice with all the passed peer addresses
// normalized with the given default port, and all duplicates removed.
func normalizeAddresses(addrs []string, defaultPort int) []string {
	for i, addr := range addrs {
		addrs[i] = normalizeAddress(addr, defaultPort)
	}

	return removeDuplicateAddresses(addrs)
}

// filesExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// checkCreateDir checks that the path exists and is a directory.
// If path does not exist, it is created.
func checkCreateDir(path string) error {
	if fi, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// Attempt data directory creation
			if err = os.MkdirAll(path, 0700); err != nil {
				return fmt.Errorf("cannot create directory: %s", err)
			}
		} else {
			return fmt.Errorf("error checking directory: %s", err)
		}
	} else {
		if !fi.IsDir() {
			return fmt.Errorf("path '%s' is not a directory", path)
		}
	}

	return nil
}

// newConfigParser returns a new command line flags parser.
func newConfigParser(cfg *Config, appName string, options flags.Options) *flags.Parser {
	p := flags.NewNamedParser(appName, options)

	if cfg != nil {
		p.AddGroup("Application Options", "", cfg)
	}

	return p
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
//      1) Start with a default config with sane settings
//      2) Pre-parse the command line to check for an alternative config file
//      3) Load configuration file overwriting defaults with any specified options
//      4) Parse CLI options and overwrite/add any specified options
//
// The above results in btcwallet functioning properly without any config
// settings while still allowing the user to override settings with config files
// and command line options.  Command line options always take precedence.
func loadConfig() (*Config, []string, error) {

	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))

	return LoadConfig(appName, os.Args[1:])
}

// LoadConfig reads a Config type from the command-line options and config file.
func LoadConfig(appName string, args []string) (*Config, []string, error) {
	// Default config.
	cfg := Config{
		DebugLevel:      defaultLogLevel,
		ConfigFile:      defaultConfigFile,
		DataDir:         defaultDataDir,
		LogDir:          defaultLogDir,
		TLSKey:          defaultTLSKeyFile,
		TLSCert:         defaultTLSCertFile,
		PowThreads:      runtime.NumCPU(),
		ProofOfWork:     defaultPowHandler,
		MsgExpiry:       defaultMsgExpiry,
		BroadcastExpiry: defaultBroadcastExpiry,
		LogConsole:      defaultLogConsole,
		GenKeys:         defaultGenKeys,
	}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.
	preCfg := cfg
	preParser := newConfigParser(&preCfg, appName, flags.HelpFlag)
	_, err := preParser.ParseArgs(args)
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
		return nil, nil, err
	}

	// Show the version and exit if the version flag was specified.
	funcName := "loadConfig"
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
	if preCfg.ShowVersion {
		fmt.Println(appName, "version", version())
		os.Exit(0)
	}

	// Load additional config from file.
	var configFileError error
	parser := newConfigParser(&cfg, appName, flags.Default)
	if preCfg.ConfigFile != defaultConfigFile || fileExists(preCfg.ConfigFile) {

		err = flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
		if err != nil {
			if _, ok := err.(*os.PathError); !ok {
				fmt.Fprintln(os.Stderr, err)
				parser.WriteHelp(os.Stderr)
				return nil, nil, err
			}
			configFileError = err
		}
	}

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.ParseArgs(args)
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			parser.WriteHelp(os.Stderr)
		}
		return nil, nil, err
	}

	// Warn about missing config file after the final command line parse
	// succeeds. This prevents the warning on help messages and invalid
	// options.
	if configFileError != nil {
		log.Warnf("%v", configFileError)
	}

	// If an alternate data directory was specified, and paths with defaults
	// relative to the data dir are unchanged, modify each path to be
	// relative to the new data dir.
	if cfg.DataDir != defaultDataDir {
		if cfg.TLSKey == defaultTLSKeyFile {
			cfg.TLSKey = filepath.Join(cfg.DataDir, "rpc.key")
		}
		if cfg.TLSCert == defaultTLSCertFile {
			cfg.TLSCert = filepath.Join(cfg.DataDir, "rpc.cert")
		}
		cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	}

	// Ensure the data directory exists.
	if err := checkCreateDir(cfg.DataDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}

	// Expand environment variable and leading ~ for filepaths.
	cfg.LogDir = cleanAndExpandPath(cfg.LogDir)

	// Special show command to list supported subsystems and exit.
	if cfg.DebugLevel == "show" {
		fmt.Println("Supported subsystems", supportedSubsystems())
		os.Exit(0)
	}

	// Initialize logging at the default logging level.
	initSeelogLogger(filepath.Join(cfg.LogDir, defaultLogFilename), cfg.LogConsole)
	setLogLevels(defaultLogLevel)

	// Parse, validate, and set debug log level(s).
	if err := parseAndSetDebugLevels(cfg.DebugLevel); err != nil {
		err := fmt.Errorf("%s: %v", "loadConfig", err.Error())
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Verify proof-of-work parameters.
	switch cfg.ProofOfWork {
	case "sequential":
		cfg.powHandler = pow.DoSequential
	case "parallel":
		if cfg.PowThreads < 2 {
			err := errors.New("Number of threads for proof-of-work cannot be less than 2")
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
		cfg.powHandler = func(target uint64, hash []byte) pow.Nonce {
			return pow.DoParallel(target, hash, cfg.PowThreads)
		}
	default:
		err := errors.New("Unknown proof-of-work handler")
		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}

	// Ensure the key file and data store exist or create them when the create
	// flag is set.
	cfg.keyfilePath = filepath.Join(cfg.DataDir, keyfileName)
	cfg.storePath = filepath.Join(cfg.DataDir, storeDbName)

	if cfg.Create {
		// Error if the create flag is set and the key file or data store
		// already exist.
		if fileExists(cfg.keyfilePath) {
			err := fmt.Errorf("The key file already exists.")
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}

		if fileExists(cfg.storePath) {
			err := fmt.Errorf("The data store already exists.")
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
		
		if cfg.GenKeys == 0 {
			cfg.GenKeys = 1
		}

		// Create databases.
		if err := createDatabases(&cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Unable to create data:", err)
			return nil, nil, err
		}

		// Created successfully, so exit now with success.
		os.Exit(0)

	} else if !fileExists(cfg.keyfilePath) || !fileExists(cfg.storePath) {
		err := errors.New("The key file and/or data store do not exist. " +
			"Run with the --create option to\ninitialize and create them.")

		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}

	// Import private keys from PyBitmessage's keys.dat file.
	if cfg.ImportKeyFile != "" {
		cfg.ImportKeyFile = cleanAndExpandPath(cfg.ImportKeyFile)

		// We need to open the keyfile and store.
		keymgr, store, _, err := openDatabases(&cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Unable to open databases:", err)
			return nil, nil, err
		}

		err = importKeyfile(keymgr, cfg.ImportKeyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}

		u := &User{keymgr, cfg.keyfilePath, cfg.Username, cfg.keyfilePass}
		u.SaveKeyfile()

		store.Close()

		// Imported successfully, so exit now with success.
		os.Exit(0)
	}

	// Username and password must be specified.
	if cfg.Username == "" || cfg.Password == "" {
		err := errors.New("Username and password cannot be left blank.")

		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}

	if cfg.RPCConnect == "" {
		cfg.RPCConnect = "127.0.0.1"
	}

	// Add default port to connect flag if missing.
	cfg.RPCConnect = normalizeAddress(cfg.RPCConnect, defaultBmdPort)

	RPCHost, _, err := net.SplitHostPort(cfg.RPCConnect)
	if err != nil {
		return nil, nil, err
	}
	if cfg.DisableClientTLS {
		if _, ok := localhostListeners[RPCHost]; !ok {
			str := "%s: the --noclienttls option may not be used when" +
				" connecting RPC to non localhost addresses: %s"
			err := fmt.Errorf(str, funcName, cfg.RPCConnect)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
	} else {
		// If CAFile is unset, choose either the copy or local bmd cert.
		if cfg.CAFile == "" {
			cfg.CAFile = filepath.Join(cfg.DataDir, defaultCAFilename)

			// If the CA copy does not exist, check if we're connecting to
			// a local bmd and switch to its RPC cert if it exists.
			if !fileExists(cfg.CAFile) {
				if _, ok := localhostListeners[RPCHost]; ok {
					if fileExists(bmdHomedirCAFile) {
						cfg.CAFile = bmdHomedirCAFile
					}
				}
			}
		}
		cfg.CAFile = cleanAndExpandPath(cfg.CAFile)
	}

	// Default RPC, IMAP, SMTP to listen on localhost only.
	addrs, err := net.LookupHost("localhost")
	if err != nil {
		return nil, nil, err
	}

	if cfg.EnableRPC && len(cfg.RPCListeners) == 0 {
		cfg.RPCListeners = make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addr = net.JoinHostPort(addr, strconv.Itoa(defaultRPCPort))
			cfg.RPCListeners = append(cfg.RPCListeners, addr)
		}
	}

	if len(cfg.IMAPListeners) == 0 {
		cfg.IMAPListeners = make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addr = net.JoinHostPort(addr, strconv.Itoa(defaultIMAPPort))
			cfg.IMAPListeners = append(cfg.IMAPListeners, addr)
		}
	}

	if len(cfg.SMTPListeners) == 0 {
		cfg.SMTPListeners = make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addr = net.JoinHostPort(addr, strconv.Itoa(defaultSMTPPort))
			cfg.SMTPListeners = append(cfg.SMTPListeners, addr)
		}
	}

	// Add default port to all RPC, IMAP and SMTP listener addresses if needed
	// and remove duplicate addresses.
	cfg.RPCListeners = normalizeAddresses(cfg.RPCListeners, defaultRPCPort)
	cfg.IMAPListeners = normalizeAddresses(cfg.IMAPListeners, defaultIMAPPort)
	cfg.SMTPListeners = normalizeAddresses(cfg.SMTPListeners, defaultSMTPPort)

	// Only allow server TLS to be disabled if the RPC is bound to localhost
	// addresses.
	if cfg.DisableServerTLS {
		err = verifyListeners(cfg.RPCListeners, "RPC", funcName, usageMessage)
		if err != nil {
			return nil, nil, err
		}
		err = verifyListeners(cfg.IMAPListeners, "IMAP", funcName, usageMessage)
		if err != nil {
			return nil, nil, err
		}
		err = verifyListeners(cfg.SMTPListeners, "SMTP", funcName, usageMessage)
		if err != nil {
			return nil, nil, err
		}
	}

	// If the bmd username or password are unset, use the same auth as for
	// the client.
	if cfg.BmdUsername == "" {
		cfg.BmdUsername = cfg.Username
	}
	if cfg.BmdPassword == "" {
		cfg.BmdPassword = cfg.Password
	}

	return &cfg, remainingArgs, nil
}
