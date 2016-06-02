// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"testing"
	"io/ioutil"
	"os"
	"strconv"
	"fmt"
)

// If config files exist while we are doing 
var oldDefaultConfigFile []byte = nil
var oldConfigFile []byte = nil
var oldConfigFilename *string = nil

func setup(defaultConfigContents, configFileContents, configFilename *string) error {
	var err error
	
	// Check if a default config file exists. If so, save it and remove it. 
	if _, err = os.Stat(defaultConfigFile); !os.IsNotExist(err) {
		oldDefaultConfigFile, err = ioutil.ReadFile(defaultConfigFile)
		
		if err != nil {
			return err
		}
		
		err = os.Remove(defaultConfigFile)
		if err != nil {
			oldDefaultConfigFile = nil
			return err
		}
	}
	
	// Check if defaultConfigContents is set. If so, make a config file. 
	if defaultConfigContents != nil {
	    err = ioutil.WriteFile(defaultConfigFile, []byte(*defaultConfigContents), 0644)
		if err != nil {
			cleanup()
			return nil
		}
	}
	
	// Check if configFilePath is set and is not equal to the default 
	// path. 
	if configFilename == nil || *configFilename == defaultConfigFile {
		return nil
	} else {
		oldConfigFilename = configFilename
	}
	
	// If the file exists, save it. 
	if _, err = os.Stat(*configFilename); !os.IsNotExist(err) {
		oldConfigFile, err = ioutil.ReadFile(*configFilename)
		
		if err != nil {
			return err
		}
		
		err = os.Remove(*configFilename)
		if err != nil {
			oldConfigFile = nil
			return err
		}
	}

	if configFileContents != nil {
		err = ioutil.WriteFile(*configFilename, []byte(*configFileContents), 0644)
		if err != nil {
			cleanup()
			return nil
		}
	}
	
	return nil
}

func cleanup() {
	if oldConfigFile == nil {
		if _, err := os.Stat(defaultConfigFile); !os.IsNotExist(err) {
			os.Remove(defaultConfigFile)
		}
	} else {
		ioutil.WriteFile(defaultConfigFile, oldDefaultConfigFile, 0644)
	}
	
	if oldConfigFilename != nil {
		if oldConfigFile == nil {
			os.Remove(*oldConfigFilename)
		} else {
			ioutil.WriteFile(*oldConfigFilename, oldDefaultConfigFile, 0644)
		}
	} 
	
	oldConfigFile = nil;
	oldConfigFilename = nil;
	oldDefaultConfigFile = nil;
}

func testConfig(t *testing.T, testId int, expected int16, cmdLine *int16, defaultConfig *int16, config *int16, configFile *string) {
	var defaultConfigContents *string
	var configFileContents *string
	var commandLine []string = []string{"-u", "admin", "-P", "admin"}
	
	defer cleanup()
	
	// first construct the command-line arguments. 
	if cmdLine != nil {
		commandLine = append(commandLine, fmt.Sprintf("--genkeys=%s", strconv.FormatInt(int64(*cmdLine), 10)))
	} 
	if configFile != nil {
		commandLine = append(commandLine, fmt.Sprintf("--configfile=%s", *configFile))
	}
	
	// Make the default config file. 
	if defaultConfig != nil {
		var dcc string = fmt.Sprintf("genkeys=%s", strconv.FormatInt(int64(*defaultConfig), 10))
		defaultConfigContents = &dcc
	}
	
	// Make the extra config file. 
	if config != nil {
		var cc string = fmt.Sprintf("genkeys=%s", strconv.FormatInt(int64(*config), 10))
		configFileContents = &cc
	}
	
	// Set up the test. 
	err := setup(defaultConfigContents, configFileContents, configFile)
	if err != nil {
		t.Fail()
	}
	
	cfg, _, err := LoadConfig("test", commandLine) 
	
	if cfg == nil {
		t.Errorf("Error, test id %d: nil config returned! %s", testId, err.Error())
		return
	}
	
	if cfg.GenKeys != expected {
		t.Errorf("Error, test id %d: expected %d got %d.", testId, expected, cfg.GenKeys)
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
	var cfg string = "altbmdagent.conf"
	testConfig(t, 3, q, nil, &q, nil, nil)
	testConfig(t, 4, q, nil, nil, &q, &cfg)
	
	// Test that an option is correctly set when specified
	// on the command line and that it overwrites the 
	// option in the config file. 
	var z int16 = 39
	testConfig(t, 5, q, &q, &z, nil, nil)
	testConfig(t, 6, q, &q, nil, &z, &cfg)
}