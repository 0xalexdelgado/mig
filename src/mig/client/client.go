// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]
package client

import (
	"bytes"
	"code.google.com/p/gcfg"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/jvehent/cljs"
	"io"
	"io/ioutil"
	"mig"
	"mig/pgp"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"
)

var version string

// A Client provides all the needed functionalities to interact with the MIG API.
// It should be initialized with a proper configuration file.
type Client struct {
	API   *http.Client
	Token string
	Conf  Configuration
}

// Configuration stores the live configuration and global parameters of a client
type Configuration struct {
	API     ApiConf
	Homedir string
	GPG     GpgConf
}

type ApiConf struct {
	URL            string
	SkipVerifyCert bool
}
type GpgConf struct {
	Home      string
	KeyID     string
	Keyserver string
}

// NewClient initiates a new instance of a Client
func NewClient(conf Configuration) Client {
	var cli Client
	cli.Conf = conf
	tr := &http.Transport{
		DisableCompression: false,
		DisableKeepAlives:  false,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS10,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
			InsecureSkipVerify: conf.API.SkipVerifyCert,
		},
	}
	cli.API = &http.Client{Transport: tr}
	return cli
}

// ReadConfiguration loads a client configuration from a local configuration file
// and verifies that GnuPG's secring is available
func ReadConfiguration(file string) (conf Configuration, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("ReadConfiguration() -> %v", e)
		}
	}()
	_, err = os.Stat(file)
	if err != nil {
		panic(err)
	}
	err = gcfg.ReadFileInto(&conf, file)
	if conf.GPG.Home == "" {
		gnupgdir := os.Getenv("GNUPGHOME")
		if gnupgdir == "" {
			gnupgdir = "/.gnupg"
		}
		conf.GPG.Home = FindHomedir() + gnupgdir
	}
	_, err = os.Stat(conf.GPG.Home + "/secring.gpg")
	if err != nil {
		panic("secring.gpg not found")
	}
	// if trailing slash is missing from API url, add it
	if conf.API.URL[len(conf.API.URL)-1] != '/' {
		conf.API.URL += "/"
	}
	return
}

func FindHomedir() string {
	if runtime.GOOS == "darwin" {
		return os.Getenv("HOME")
	} else {
		// find keyring in default location
		u, err := user.Current()
		if err != nil {
			panic(err)
		}
		return u.HomeDir
	}
}

// Do is a thin wrapper around http.Client.Do() that inserts an authentication header
// to the outgoing request
func (cli Client) Do(r *http.Request) (resp *http.Response, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("Do() -> %v", e)
		}
	}()
	r.Header.Set("User-Agent", "MIG Client v"+version)
	if cli.Token == "" {
		cli.Token, err = cli.MakeSignedToken()
		if err != nil {
			panic(err)
		}
	}
	r.Header.Set("X-PGPAUTHORIZATION", cli.Token)
	// execute the request
	resp, err = cli.API.Do(r)
	if err != nil {
		panic(err)
	}
	// if the request failed because of an auth issue, it may be that the auth token has expired.
	// try the request again with a fresh token
	if resp.StatusCode == 401 {
		resp.Body.Close()
		cli.Token, err = cli.MakeSignedToken()
		if err != nil {
			panic(err)
		}
		r.Header.Set("X-PGPAUTHORIZATION", cli.Token)
		// execute the request
		resp, err = cli.API.Do(r)
		if err != nil {
			panic(err)
		}
	}
	return
}

// GetAPIResource retrieves a cljs resource from a target endpoint. The target must be the relative
// to the API URL passed in the configuration. For example, if the API URL is `http://localhost:12345/api/v1/`
// then target could only be set to `dashboard` to retrieve `http://localhost:12345/api/v1/dashboard`
func (cli Client) GetAPIResource(target string) (resource *cljs.Resource, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("GetAPIResource() -> %v", e)
		}
	}()
	r, err := http.NewRequest("GET", cli.Conf.API.URL+target, nil)
	if err != nil {
		panic(err)
	}
	resp, err := cli.Do(r)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	// unmarshal the body. don't attempt to interpret it, as long as it
	// fits into a cljs.Resource, it's acceptable
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	if len(body) > 1 {
		err = json.Unmarshal(body, &resource)
		if err != nil {
			panic(err)
		}
	}
	if resp.StatusCode != 200 {
		err = fmt.Errorf("error: HTTP %d. API call failed with error '%v' (code %s)",
			resp.StatusCode, resource.Collection.Error.Message, resource.Collection.Error.Code)
		panic(err)
	}
	return
}

// GetAction retrieves a MIG Action from the API using its Action ID
func (cli Client) GetAction(aid float64) (a mig.Action, links []cljs.Link, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("GetAction() -> %v", e)
		}
	}()
	target := fmt.Sprintf("action?actionid=%.0f", aid)
	resource, err := cli.GetAPIResource(target)
	if err != nil {
		panic(err)
	}
	if resource.Collection.Items[0].Data[0].Name != "action" {
		panic("API returned something that is not an action... something's wrong.")
	}
	a, err = ValueToAction(resource.Collection.Items[0].Data[0].Value)
	if err != nil {
		panic(err)
	}
	links = resource.Collection.Items[0].Links
	return
}

// PostAction submits a MIG Action to the API and returns the reflected action with API ID
func (cli Client) PostAction(a mig.Action) (a2 mig.Action, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("PostAction() -> %v", e)
		}
	}()
	// serialize
	ajson, err := json.Marshal(a)
	if err != nil {
		panic(err)
	}
	actionstr := string(ajson)
	data := url.Values{"action": {actionstr}}
	r, err := http.NewRequest("POST", cli.Conf.API.URL+"action/create/", strings.NewReader(data.Encode()))
	if err != nil {
		panic(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := cli.Do(r)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if err != nil {
		panic(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != 202 {
		err = fmt.Errorf("error: HTTP %d. action creation failed.", resp.StatusCode)
		panic(err)
	}
	var resource *cljs.Resource
	err = json.Unmarshal(body, &resource)
	if err != nil {
		panic(err)
	}
	a2, err = ValueToAction(resource.Collection.Items[0].Data[0].Value)
	if err != nil {
		panic(err)
	}
	return
}

func ValueToAction(v interface{}) (a mig.Action, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("ValueToAction() -> %v", e)
		}
	}()
	bData, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(bData, &a)
	if err != nil {
		panic(err)
	}
	return
}

func (cli Client) GetCommand(cmdid float64) (cmd mig.Command, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("GetCommand() -> %v", e)
		}
	}()
	target := "command?commandid=" + fmt.Sprintf("%.0f", cmdid)
	resource, err := cli.GetAPIResource(target)
	if err != nil {
		panic(err)
	}
	if resource.Collection.Items[0].Data[0].Name != "command" {
		panic("API returned something that is not a command... something's wrong.")
	}
	cmd, err = ValueToCommand(resource.Collection.Items[0].Data[0].Value)
	if err != nil {
		panic(err)
	}
	return
}

func ValueToCommand(v interface{}) (cmd mig.Command, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("ValueToCommand() -> %v", e)
		}
	}()
	bData, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(bData, &cmd)
	if err != nil {
		panic(err)
	}
	return
}

func (cli Client) GetAgent(agtid float64) (agt mig.Agent, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("GetAgent() -> %v", e)
		}
	}()
	target := "agent?agentid=" + fmt.Sprintf("%.0f", agtid)
	resource, err := cli.GetAPIResource(target)
	if err != nil {
		panic(err)
	}
	if resource.Collection.Items[0].Data[0].Name != "agent" {
		panic("API returned something that is not an agent... something's wrong.")
	}
	agt, err = ValueToAgent(resource.Collection.Items[0].Data[0].Value)
	if err != nil {
		panic(err)
	}
	return
}

func ValueToAgent(v interface{}) (agt mig.Agent, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("valueToAgent() -> %v", e)
		}
	}()
	bData, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(bData, &agt)
	if err != nil {
		panic(err)
	}
	return
}

func (cli Client) GetInvestigator(iid float64) (inv mig.Investigator, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("GetInvestigator() -> %v", e)
		}
	}()
	target := "investigator?investigatorid=" + fmt.Sprintf("%.0f", iid)
	resource, err := cli.GetAPIResource(target)
	if err != nil {
		panic(err)
	}
	if resource.Collection.Items[0].Data[0].Name != "investigator" {
		panic("API returned something that is not an investigator... something's wrong.")
	}
	inv, err = ValueToInvestigator(resource.Collection.Items[0].Data[0].Value)
	if err != nil {
		panic(err)
	}
	return
}

// PostInvestigator creates an Investigator and returns the reflected investigator
func (cli Client) PostInvestigator(name string, pubkey []byte) (inv mig.Investigator, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("PostInvestigator() -> %v", e)
		}
	}()
	// build the body into buf using a multipart writer
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	// set the name form value
	err = writer.WriteField("name", name)
	if err != nil {
		panic(err)
	}
	// set the publickey form value
	part, err := writer.CreateFormFile("publickey", fmt.Sprintf("%s.asc", name))
	if err != nil {
		panic(err)
	}
	_, err = io.Copy(part, bytes.NewReader(pubkey))
	if err != nil {
		panic(err)
	}
	// must close the writer to write trailing boundary
	err = writer.Close()
	if err != nil {
		panic(err)
	}
	// post the request
	r, err := http.NewRequest("POST", cli.Conf.API.URL+"investigator/create/", buf)
	if err != nil {
		panic(err)
	}
	r.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := cli.Do(r)
	if err != nil {
		panic(err)
	}
	// get the investigator back from the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	var resource *cljs.Resource
	err = json.Unmarshal(body, &resource)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != 201 {
		err = fmt.Errorf("HTTP %d: %v (code %s)", resp.StatusCode,
			resource.Collection.Error.Message, resource.Collection.Error.Code)
		return
	}
	inv, err = ValueToInvestigator(resource.Collection.Items[0].Data[0].Value)
	if err != nil {
		panic(err)
	}
	return
}

// PostInvestigatorStatus updates the status of an Investigator
func (cli Client) PostInvestigatorStatus(iid float64, newstatus string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("PostInvestigatorStatus() -> %v", e)
		}
	}()
	data := url.Values{"id": {fmt.Sprintf("%.0f", iid)}, "status": {newstatus}}
	r, err := http.NewRequest("POST", cli.Conf.API.URL+"investigator/update/", strings.NewReader(data.Encode()))
	if err != nil {
		panic(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := cli.Do(r)
	if err != nil {
		panic(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	var resource *cljs.Resource
	if len(body) > 1 {
		err = json.Unmarshal(body, &resource)
		if err != nil {
			panic(err)
		}
	}
	if resp.StatusCode != 200 {
		err = fmt.Errorf("error: HTTP %d. status update failed with error '%v' (code %s)",
			resp.StatusCode, resource.Collection.Error.Message, resource.Collection.Error.Code)
		panic(err)
	}
	return
}

func ValueToInvestigator(v interface{}) (inv mig.Investigator, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("valueToInvestigator) -> %v", e)
		}
	}()
	bData, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(bData, &inv)
	if err != nil {
		panic(err)
	}
	return
}

// MakeSignedToken encrypts a timestamp and a random number with the users GPG key
// to use as an auth token with the API
func (cli Client) MakeSignedToken() (token string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("MakeSignedToken() -> %v", e)
		}
	}()
	tokenVersion := 1
	str := fmt.Sprintf("%d;%s;%.0f", tokenVersion, time.Now().UTC().Format(time.RFC3339), mig.GenID())
	secringFile, err := os.Open(cli.Conf.GPG.Home + "/secring.gpg")
	if err != nil {
		panic(err)
	}
	sig, err := pgp.Sign(str+"\n", cli.Conf.GPG.KeyID, secringFile)
	if err != nil {
		panic(err)
	}
	token = str + ";" + sig
	return
}

// SignAction takes a MIG Action, signs it with the key identified in the configuration
// and returns the signed action
func (cli Client) SignAction(a mig.Action) (signed_action mig.Action, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("SignAction() -> %v", e)
		}
	}()
	filename, err := a.ToTempFile()
	if err != nil {
		panic(err)
	}
	a2, err := mig.ActionFromFile(filename)
	if err != nil {
		panic(err)
	}
	secring, err := os.Open(cli.Conf.GPG.Home + "/secring.gpg")
	if err != nil {
		panic(err)
	}
	defer secring.Close()
	sig, err := a2.Sign(cli.Conf.GPG.KeyID, secring)
	if err != nil {
		panic(err)
	}
	a.PGPSignatures = append(a.PGPSignatures, sig)
	signed_action = a
	return
}

// EvaluateAgentTarget runs a search against the api to find all agents that match an action target string
func (cli Client) EvaluateAgentTarget(target string) (agents []mig.Agent, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("EvaluateAgentTarget() -> %v", e)
		}
	}()
	query := "search?type=agent&target=" + url.QueryEscape(target)
	resource, err := cli.GetAPIResource(query)
	if err != nil {
		panic(err)
	}
	for _, item := range resource.Collection.Items {
		for _, data := range item.Data {
			if data.Name != "agent" {
				continue
			}
			agt, err := ValueToAgent(data.Value)
			if err != nil {
				panic(err)
			}
			agents = append(agents, agt)
		}
	}
	return
}
