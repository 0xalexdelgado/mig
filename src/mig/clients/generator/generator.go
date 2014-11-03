// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"mig"
	"mig/pgp"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"runtime"
	"time"
)

func main() {

	var Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Mozilla InvestiGator Action Generator\n"+
				"usage: %s -k=<key id> (-i <input file)\n\n"+
				"Command line to generate and sign MIG Actions.\n"+
				"The resulting actions are display on stdout.\n\n"+
				"Options:\n",
			os.Args[0])
		flag.PrintDefaults()
	}

	// command line options
	var key = flag.String("k", "key identifier", "Key identifier used to sign the action (ex: B75C2346)")
	var pretty = flag.Bool("p", false, "Print signed action in pretty JSON format")
	var urlencode = flag.Bool("urlencode", false, "URL Encode marshalled JSON before output")
	var posturl = flag.String("posturl", "", "POST action to <url> (enforces urlencode)")
	var file = flag.String("i", "/path/to/file", "Load action from file")
	var target = flag.String("t", "some.target.example.net", "Set the target of the action")
	var validfrom = flag.String("validfrom", "now", "(optional) set an ISO8601 date the action will be valid from. If unset, use 'now'.")
	var expireafter = flag.String("expireafter", "30m", "(optional) set a validity duration for the action. If unset, use '30m'.")
	flag.Parse()

	// We need a key, if none is set on the command line, fail
	if *key == "key identifier" {
		Usage()
		os.Exit(-1)
	}

	var err error

	// if a file is defined, load action from that
	if *file == "/path/to/file" {
		fmt.Println("Missing action file")
		os.Exit(1)
	}
	a, err := mig.ActionFromFile(*file)
	if err != nil {
		panic(err)
	}

	// set the dates
	if *validfrom == "now" {
		// for immediate execution, set validity one minute in the past
		a.ValidFrom = time.Now().Add(-60 * time.Second).UTC()
	} else {
		a.ValidFrom, err = time.Parse(time.RFC3339, *validfrom)
		if err != nil {
			panic(err)
		}
	}
	period, err := time.ParseDuration(*expireafter)
	if err != nil {
		log.Fatal(err)
	}
	a.ExpireAfter = a.ValidFrom.Add(period)

	if *target != "some.target.example.net" {
		a.Target = *target
	}

	// find homedir
	var homedir string
	if runtime.GOOS == "darwin" {
		homedir = os.Getenv("HOME")
	} else {
		// find keyring in default location
		u, err := user.Current()
		if err != nil {
			panic(err)
		}
		homedir = u.HomeDir
	}
	// load keyrings
	var gnupghome string
	gnupghome = os.Getenv("GNUPGHOME")
	if gnupghome == "" {
		gnupghome = "/.gnupg"
	}
	pubringFile, err := os.Open(homedir + gnupghome + "/pubring.gpg")

	if err != nil {
		panic(err)
	}
	defer pubringFile.Close()

	secringFile, err := os.Open(homedir + gnupghome + "/secring.gpg")
	if err != nil {
		panic(err)
	}
	defer secringFile.Close()

	// compute the signature
	str, err := a.String()
	if err != nil {
		panic(err)
	}
	pgpsig, err := pgp.Sign(str, *key, secringFile)
	if err != nil {
		panic(err)
	}

	// store the signature in the action signature array
	a.PGPSignatures = append(a.PGPSignatures, pgpsig)

	// syntax checking
	err = a.Validate()
	if err != nil {
		panic(err)
	}

	// signature checking
	err = a.VerifySignatures(pubringFile)
	if err != nil {
		panic(err)
	}

	// if asked, pretty print the action
	var jsonAction []byte
	if *pretty {
		jsonAction, err = json.MarshalIndent(a, "", "\t")
		fmt.Printf("%s\n", jsonAction)
	} else {
		jsonAction, err = json.Marshal(a)
	}
	if err != nil {
		panic(err)
	}

	// if asked, url encode the action before marshaling it
	actionstr := string(jsonAction)
	if *urlencode {
		strJsonAction := string(jsonAction)
		actionstr = url.QueryEscape(strJsonAction)
		if *pretty {
			fmt.Println(actionstr)
		}
	}

	// http post the action to the posturl endpoint
	if *posturl != "" {
		resp, err := http.PostForm(*posturl, url.Values{"action": {actionstr}})
		defer resp.Body.Close()
		if err != nil {
			panic(err)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s", body)
	}
}
