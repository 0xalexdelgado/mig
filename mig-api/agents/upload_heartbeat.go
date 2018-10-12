// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Zack Mullaly zmullaly@mozilla.com [:zack]

package agents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mozilla/mig"
)

const timeToExpireHeartbeat = 5 * time.Minute

// PersistHeartbeat abstracts over operations that allow the MIG API to
// save some information about agents sent in a heartbeat message.
type PersistHeartbeat interface {
	PersistHeartbeat(Heartbeat) error
}

// Authenticator abstracts over operations that authenticate agents to
// determine whether an agent should be allowed to persist a heartbeat.
type Authenticator interface {
	Authenticate(Heartbeat) error
}

// UploadHeartbeat is an HTTP request handler that serves POST requests
// containing a Heartbeat encoded as JSON.
type UploadHeartbeat struct {
	persist PersistHeartbeat
	auth    Authenticator
}

// NewUploadHeartbeat constructs a new UploadHeartbeat.
func NewUploadHeartbeat(persist PersistHeartbeat, auth Authenticator) UploadHeartbeat {
	return UploadHeartbeat{
		persist: persist,
		auth:    auth,
	}
}

// Environment contains information about the environment an agent is running in.
type Environment struct {
	Init      string   `json:"init"`
	Ident     string   `json:"ident"`
	OS        string   `json:"os"`
	Arch      string   `json:"arch"`
	IsProxied bool     `json:"isProxied"`
	Proxy     string   `json:"proxy"`
	Addresses []string `json:"addresses"`
	PublicIP  string   `json:"publicIP"`
	Modules   []string `json:"modules"`
}

// Tag is a label associated with an agent.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Heartbeat contains information describing an active agent.
type Heartbeat struct {
	Name        string      `json:"name"`
	Mode        string      `json:"mode"`
	Version     string      `json:"version"`
	PID         uint        `json:"pid"`
	QueueLoc    string      `json:"queueLoc"`
	StartTime   time.Time   `json:"startTime"`
	Environment Environment `json:"environment"`
	Tags        []Tag       `json:"tags"`
}

type uploadHeartbeatResponse struct {
	Error *string `json:"error"`
}

// validate ensures that a heartbeat message contains reasonable-looking data.
// Most of the utility of this function is just in making sure that all of the fields
// are populated.  Go will decode JSON missing some of the required fields and supply
// zero values (such as "" for strings) instead of erroring, which is not what we want.
func (hb Heartbeat) validate() error {
	missingFields := map[string]bool{
		"name":                 hb.Name == "",
		"mode":                 hb.Mode == "",
		"version":              hb.Version == "",
		"queueLoc":             hb.QueueLoc == "",
		"pid":                  hb.PID == 0,
		"environment.init":     hb.Environment.Init == "",
		"environment.ident":    hb.Environment.Ident == "",
		"environment.os":       hb.Environment.OS == "",
		"environment.arch":     hb.Environment.Arch == "",
		"environment.publicIP": hb.Environment.PublicIP == "",
	}

	missing := []string{}

	for fieldName, isMissing := range missingFields {
		if isMissing {
			missing = append(missing, fieldName)
		}
	}

	if len(missing) != 0 {
		return fmt.Errorf("missing field(s): %s", strings.Join(missing, ", "))
	}

	// After checking that we have all the fields we want, we will check that those fields
	// appear to contain sensible values.
	isExpired := time.Now().Add(-timeToExpireHeartbeat).After(hb.StartTime)
	hasBadMode := hb.Mode != "" && hb.Mode != "daemon" && hb.Mode != "checkin"
	nameTooLong := len(hb.Name) > 1024
	versionTooLong := len(hb.Version) > 128
	errorConditions := map[bool]string{
		isExpired:      "heartbeat expired",
		hasBadMode:     "agent reported an invalid mode",
		nameTooLong:    "agent reported a name that is too long",
		versionTooLong: "agent reported a version string that is too long",
	}

	for didError, errMsg := range errorConditions {
		if didError {
			return fmt.Errorf(errMsg)
		}
	}

	return nil
}

func (handler UploadHeartbeat) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	reqData := Heartbeat{}
	decoder := json.NewDecoder(request.Body)
	resEncoder := json.NewEncoder(response)

	response.Header().Set("Content-Type", "application/json")

	defer request.Body.Close()

	decodeErr := decoder.Decode(&reqData)
	if decodeErr != nil {
		errMsg := fmt.Sprintf("Failed to decode request body: %s", decodeErr.Error())
		response.WriteHeader(http.StatusBadRequest)
		resEncoder.Encode(&uploadHeartbeatResponse{&errMsg})
		return
	}

	validateErr := reqData.validate()
	if validateErr != nil {
		errMsg := fmt.Sprintf("Missing fields in request body: %s", validateErr.Error())
		response.WriteHeader(http.StatusBadRequest)
		resEncoder.Encode(&uploadHeartbeatResponse{&errMsg})
		return
	}

	authErr := handler.auth.Authenticate(reqData)
	if authErr != nil {
		errMsg := fmt.Sprintf("Agent is not authorized to upload heartbeats: %s", authErr.Error())
		response.WriteHeader(http.StatusUnauthorized)
		resEncoder.Encode(&uploadHeartbeatResponse{&errMsg})
		return
	}

	persistErr := handler.persist.PersistHeartbeat(reqData)
	if persistErr != nil {
		errMsg := fmt.Sprintf("Failed to save heartbeat: %s", persistErr.Error())
		response.WriteHeader(http.StatusInternalServerError)
		resEncoder.Encode(&uploadHeartbeatResponse{&errMsg})
		return
	}

	resEncoder.Encode(&uploadHeartbeatResponse{nil})
}

func (hb Heartbeat) ToMigAgent() mig.Agent {
	tags := make(map[string]string)
	for _, tag := range hb.Tags {
		tags[tag.Name] = tag.Value
	}

	return mig.Agent{
		Name:      hb.Name,
		Mode:      hb.Mode,
		QueueLoc:  hb.QueueLoc,
		Version:   hb.Version,
		PID:       int(hb.PID),
		StartTime: hb.StartTime,
		Env: mig.AgentEnv{
			Init:      hb.Environment.Init,
			Ident:     hb.Environment.Ident,
			OS:        hb.Environment.OS,
			Arch:      hb.Environment.Arch,
			IsProxied: hb.Environment.IsProxied,
			Proxy:     hb.Environment.Proxy,
			Addresses: hb.Environment.Addresses,
			PublicIP:  hb.Environment.PublicIP,
			Modules:   hb.Environment.Modules,
		},
		Tags: tags,
	}
}
