// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]
package main

import (
	"bufio"
	"fmt"
	"mig"
	"os"
	"regexp"
)

// If a whitelist is defined, lookup the agent in it, and return nil if found, or error if not
func isAgentAuthorized(agentQueueLoc string, ctx Context) (ok bool, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("isAgentAuthorized() -> %v", e)
		}
		ctx.Channels.Log <- mig.Log{OpID: ctx.OpID, Desc: "leaving isAgentAuthorized()"}.Debug()
	}()
	var re *regexp.Regexp
	// bypass mode if there's no whitelist in the conf
	if ctx.Agent.Whitelist == "" {
		ctx.Channels.Log <- mig.Log{OpID: ctx.OpID, Desc: "Agent authorization checking is disabled"}.Debug()
		return
	}

	wfd, err := os.Open(ctx.Agent.Whitelist)
	if err != nil {
		panic(err)
	}
	defer wfd.Close()

	scanner := bufio.NewScanner(wfd)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			panic(err)
		}
		re, err = regexp.Compile("^" + scanner.Text() + "$")
		if err != nil {
			panic(err)
		}
		if re.MatchString(agentQueueLoc) {
			ctx.Channels.Log <- mig.Log{OpID: ctx.OpID, Desc: fmt.Sprintf("Agent '%s' is authorized", agentQueueLoc)}.Debug()
			ok = true
			return
		}
	}
	// whitelist check failed, agent isn't authorized
	return
}
