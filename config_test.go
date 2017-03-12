// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// resetCfg is called to refresh configuration before every test. The returned
// function is supposed to be called at the end of the test; to clear temp
// directories.
func resetCfg(cfg *Config) func() {
	dir, err := ioutil.TempDir("", "bmagent")
	if err != nil {
		panic(fmt.Sprint("Failed to create temporary directory:", err))
	}
	cfg.DataDir = dir
	cfg.LogDir = filepath.Join(cfg.DataDir, defaultLogDirname)

	return func() {
		os.RemoveAll(dir)
	}
}

func setup(dataDir string, defaultConfigContents, configFileContents, configFilename *string) error {
	var err error

	defaultFile := filepath.Join(dataDir, defaultConfigFilename)

	// Check if defaultConfigContents is set. If so, make a config file.
	if defaultConfigContents != nil {
		err = ioutil.WriteFile(defaultFile, []byte(*defaultConfigContents), 0644)
		if err != nil {
			return nil
		}
	}

	// Check if configFilePath is set and is not equal to the default
	// path.
	if configFilename == nil || *configFilename == defaultFile {
		return nil
	}

	configFile := filepath.Join(dataDir, *configFilename)

	// If the file exists, remove it.
	if _, err = os.Stat(configFile); !os.IsNotExist(err) {
		err = os.Remove(configFile)
		if err != nil {
			return err
		}
	}

	if configFileContents != nil {
		err = ioutil.WriteFile(configFile, []byte(*configFileContents), 0644)
		if err != nil {
			return nil
		}
	}

	return nil
}

func testConfig(t *testing.T, testID int, expected int16, cmdLine *int16, defaultConfig *int16, config *int16, configFile *string) {
	var defaultConfigContents *string
	var configFileContents *string
	commandLine := []string{"-u", "admin", "-P", "admin"}

	// Ensures that the temp directory is deleted.
	cfg := DefaultConfig()
	defer resetCfg(cfg)()

	// first construct the command-line arguments.
	if cmdLine != nil {
		commandLine = append(commandLine, fmt.Sprintf("--genkeys=%s", strconv.FormatInt(int64(*cmdLine), 10)))
	}
	if configFile != nil {
		commandLine = append(commandLine, fmt.Sprintf("--configfile=%s", *configFile))
	}

	// Make the default config file.
	if defaultConfig != nil {
		dcc := fmt.Sprintf("genkeys=%s", strconv.FormatInt(int64(*defaultConfig), 10))
		defaultConfigContents = &dcc
	}

	// Make the extra config file.
	if config != nil {
		cc := fmt.Sprintf("genkeys=%s", strconv.FormatInt(int64(*config), 10))
		configFileContents = &cc
	}

	// Set up the test.
	err := setup(cfg.DataDir, defaultConfigContents, configFileContents, configFile)
	if err != nil {
		t.Fail()
	}

	remaining, err := LoadConfig("test", cfg, commandLine)
	if len(remaining) != 0 {
		t.Errorf("Remaining options not read: %v", remaining)
	}

	if err != nil {
		t.Errorf("Error, test id %d: nil config returned! %s", testID, err.Error())
		return
	}

	if cfg.GenKeys != expected {
		t.Errorf("Error, test id %d: expected %d got %d.", testID, expected, cfg.GenKeys)
	}

	return
}

func TestLoadConfig(t *testing.T) {

	// Test that an option is correctly set by default when
	// no such option is specified in the default config file
	// or on the command line.
	testConfig(t, 1, defaultGenKeys, nil, nil, nil, nil)

	// Test that an option is correctly set when specified
	// on the command line.
	var q int16 = 97
	testConfig(t, 2, q, &q, nil, nil, nil)

	// Test that an option is correctly set when specified
	// in the default config file without a command line
	// option set.
	file := "altbmdagent.conf"
	testConfig(t, 3, q, nil, &q, nil, nil)
	testConfig(t, 4, q, nil, nil, &q, &file)

	// Test that an option is correctly set when specified
	// on the command line and that it overwrites the
	// option in the config file.
	var z int16 = 39
	testConfig(t, 5, q, &q, &z, nil, nil)
	testConfig(t, 6, q, &q, nil, &z, &file)
}
