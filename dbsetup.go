// Originally derived from: btcsuite/btcwallet/walletsetup.go
// Copyright (c) 2013-2014 The btcsuite developers

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/store"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	consoleReader = bufio.NewReader(os.Stdin)
)

// promptConsoleList prompts the user with the given prefix, list of valid
// responses, and default list entry to use.  The function will repeat the
// prompt to the user until they enter a valid response.
func promptConsoleList(prefix string, validResponses []string, defaultEntry string) (string, error) {
	// Setup the prompt according to the parameters.
	validStrings := strings.Join(validResponses, "/")
	var prompt string
	if defaultEntry != "" {
		prompt = fmt.Sprintf("%s (%s) [%s]: ", prefix, validStrings,
			defaultEntry)
	} else {
		prompt = fmt.Sprintf("%s (%s): ", prefix, validStrings)
	}

	// Prompt the user until one of the valid responses is given.
	for {
		fmt.Print(prompt)
		reply, err := consoleReader.ReadString('\n')
		if err != nil {
			return "", err
		}
		reply = strings.TrimSpace(strings.ToLower(reply))
		if reply == "" {
			reply = defaultEntry
		}

		for _, validResponse := range validResponses {
			if reply == validResponse {
				return reply, nil
			}
		}
	}
}

// promptConsoleListBool prompts the user for a boolean (yes/no) with the given
// prefix. The function will repeat the prompt to the user until they enter a
// valid reponse.
func promptConsoleListBool(prefix string, defaultEntry string) (bool, error) {
	// Setup the valid responses.
	valid := []string{"n", "no", "y", "yes"}
	response, err := promptConsoleList(prefix, valid, defaultEntry)
	if err != nil {
		return false, err
	}
	return response == "yes" || response == "y", nil
}

// promptConsolePass prompts the user for a passphrase with the given prefix.
// The function will ask the user to confirm the passphrase and will repeat
// the prompts until they enter a matching response.
func promptConsolePass(prefix string, confirm bool) ([]byte, error) {
	// Prompt the user until they enter a passphrase.
	prompt := fmt.Sprintf("%s: ", prefix)
	for {
		fmt.Print(prompt)
		pass, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return nil, err
		}
		fmt.Print("\n")
		pass = bytes.TrimSpace(pass)
		if len(pass) == 0 {
			continue
		}

		if !confirm {
			return pass, nil
		}

		fmt.Print("Confirm passphrase: ")
		confirm, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return nil, err
		}
		fmt.Print("\n")
		confirm = bytes.TrimSpace(confirm)
		if !bytes.Equal(pass, confirm) {
			fmt.Println("The entered passphrases do not match.")
			continue
		}

		return pass, nil
	}
}

// promptKeyfilePassPhrase is used to prompt for the passphrase required to
// decrypt the key file.
func promptKeyfilePassPhrase() ([]byte, error) {
	prompt := "Enter the private passphrase of your key file: "
	return promptConsolePass(prompt, false)
}

// promptStorePassPhrase is used to prompt for the passphrase required to
// decrypt the data store.
func promptStorePassPhrase() ([]byte, error) {
	prompt := "Enter the private passphrase of your data store: "
	return promptConsolePass(prompt, false)
}

// promptConsoleSeed prompts the user whether they want to use an existing
// Bitmessage address generation seed. When the user answers no, a seed will be
// generated and displayed to the user along with prompting them for
// confirmation. When the user answers yes, the user is prompted for it. All
// prompts are repeated until the user enters a valid response.
func promptConsoleSeed() ([]byte, error) {
	// Ascertain the wallet generation seed.
	useUserSeed, err := promptConsoleListBool("\nDo you have an "+
		"existing Bitmessage address generation seed you want to use?", "no")
	if err != nil {
		return nil, err
	}
	if !useUserSeed {
		seed, err := hdkeychain.GenerateSeed(hdkeychain.RecommendedSeedLen)
		if err != nil {
			return nil, err
		}

		fmt.Println("\nYour address generation seed is:")
		fmt.Printf("%x\n\n", seed)
		fmt.Println("IMPORTANT: Please keep in mind that anyone who has" +
			" access to the seed can also restore your addresses thereby " +
			"giving them access to all your Bitmessage identities, so it is " +
			"imperative that you keep it in a secure location.\n")

		for {
			fmt.Print(`Once you have stored the seed in a safe ` +
				`and secure location, enter "OK" to continue: `)
			confirmSeed, err := consoleReader.ReadString('\n')
			if err != nil {
				return nil, err
			}
			confirmSeed = strings.TrimSpace(confirmSeed)
			confirmSeed = strings.Trim(confirmSeed, `"`)
			if confirmSeed == "OK" {
				break
			}
		}

		return seed, nil
	}

	for {
		fmt.Print("Enter existing address generation seed: ")
		seedStr, err := consoleReader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		seedStr = strings.TrimSpace(strings.ToLower(seedStr))

		seed, err := hex.DecodeString(seedStr)
		if err != nil || len(seed) < hdkeychain.MinSeedBytes ||
			len(seed) > hdkeychain.MaxSeedBytes {

			fmt.Printf("Invalid seed specified.  Must be a "+
				"hexadecimal value that is at least %d bits and "+
				"at most %d bits\n", hdkeychain.MinSeedBytes*8,
				hdkeychain.MaxSeedBytes*8)
			continue
		}

		return seed, nil
	}
}

// createDatabases prompts the user for information needed to generate a new
// key file and data store and generates them accordingly. The new databases
// will reside at the provided path.
func createDatabases(cfg *config) error {
	// Start by prompting for the private passphrase for the key file.
	prompt := "Enter passphrase for the key file"
	keyfilePass, err := promptConsolePass(prompt, true)
	if err != nil {
		return err
	}

	// Ascertain the address generation seed. This will either be an
	// automatically generated value the user has already confirmed or a value
	// the user has entered which has already been validated.
	seed, err := promptConsoleSeed()
	if err != nil {
		return err
	}

	// Prompt for the private passphrase for the data store.
	prompt = "\nEnter passphrase for the data store"
	storePass, err := promptConsolePass(prompt, true)

	// Create the key file.
	fmt.Println("\nCreating the key file...")

	// Intialize key manager with seed.
	kmgr, err := keymgr.New(seed)
	if err != nil {
		return err
	}

	// Save key file to disk with the specified passphrase.
	enc, err := kmgr.SaveEncrypted(keyfilePass)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(cfg.keyfilePath, enc, 0600)
	if err != nil {
		return err
	}

	// Create the data store.
	fmt.Println("Creating the data store...")
	s, err := store.Open(cfg.storePath, storePass)
	if err != nil {
		return fmt.Errorf("Failed to create data store: %v", err)
	}
	err = s.Close()
	if err != nil {
		return err
	}

	fmt.Println("The key file and data store have successfully been created.")
	return nil
}

// openDatabases returns an instance of keymgr.Manager, and store.Store based on
// the configuration.
func openDatabases() (*keymgr.Manager, *store.Store, error) {
	// Read key file passphrase from console.
	keyfilePass, err := promptConsolePass("Enter key file passphrase", false)
	if err != nil {
		return nil, nil, err
	}

	// Read store passphrase from console.
	storePass, err := promptConsolePass("Enter data store passphrase", false)
	if err != nil {
		return nil, nil, err
	}

	// Read encrypted key file.
	enc, err := ioutil.ReadFile(cfg.keyfilePath)
	if err != nil {
		return nil, nil, err
	}

	// Create an instance of key manager.
	kmgr, err := keymgr.FromEncrypted(enc, keyfilePass)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create key manager: %v", err)
	}

	// Open store.
	dstore, err := store.Open(cfg.storePath, storePass)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to open data store: %v", err)
	}

	return kmgr, dstore, nil
}
