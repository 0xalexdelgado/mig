// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"mig"
	"strconv"
	"strings"
	"time"

	"github.com/bobappleyard/readline"
)

// agentReader retrieves an agent from the api
// and enters prompt mode to analyze it
func agentReader(input string, ctx Context) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("agentReader() -> %v", e)
		}
	}()
	inputArr := strings.Split(input, " ")
	if len(inputArr) < 2 {
		panic("wrong order format. must be 'agent <agentid>'")
	}
	agtid := inputArr[1]
	agt, err := getAgent(agtid, ctx)
	if err != nil {
		panic(err)
	}

	fmt.Println("Entering agent reader mode. Type \x1b[32;1mexit\x1b[0m or press \x1b[32;1mctrl+d\x1b[0m to leave. \x1b[32;1mhelp\x1b[0m may help.")
	agtname := agt.Name
	if useShortNames {
		agtname = shorten(agtname)
	}
	fmt.Printf("Agent %.0f named '%s'\n", agt.ID, agtname)
	prompt := "\x1b[34;1magent " + agtid[len(agtid)-3:len(agtid)] + ">\x1b[0m "
	for {
		// completion
		var symbols = []string{"details", "exit", "help", "json", "pretty", "r", "lastactions"}
		readline.Completer = func(query, ctx string) []string {
			var res []string
			for _, sym := range symbols {
				if strings.HasPrefix(sym, query) {
					res = append(res, sym)
				}
			}
			return res
		}

		input, err := readline.String(prompt)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("error: ", err)
			break
		}
		orders := strings.Split(input, " ")
		switch orders[0] {
		case "details":
			agt, err = getAgent(agtid, ctx)
			if err != nil {
				panic(err)
			}
			location := agt.QueueLoc
			if useShortNames {
				location = shorten(location)
			}
			fmt.Printf(`Agent ID %.0f
name       %s
last seen  %s ago
version    %s
location   %s
os         %s
pid        %d
starttime  %s
status     %s
`, agt.ID, agtname, time.Now().Sub(agt.HeartBeatTS).String(), agt.Version, location, agt.OS, agt.PID, agt.StartTime, agt.Status)
		case "exit":
			fmt.Printf("exit\n")
			goto exit
		case "help":
			fmt.Printf(`The following orders are available:
details			print the details of the agent
exit			exit this mode
help			show this help
json <pretty>		show the json of the agent registration
r			refresh the agent (get latest version from upstream)
lastactions <limit>	print the last actions that ran on the agent. limit=10 by default.
`)
		case "lastactions":
			limit := 10
			if len(orders) > 1 {
				limit, err = strconv.Atoi(orders[1])
				if err != nil {
					panic(err)
				}
			}
			err = printAgentLastActions(agtid, limit)
			if err != nil {
				panic(err)
			}
		case "json":
			var agtjson []byte
			if len(orders) > 1 {
				if orders[1] == "pretty" {
					agtjson, err = json.MarshalIndent(agt, "", "  ")
				} else {
					fmt.Printf("Unknown option '%s'\n", orders[1])
				}
			} else {
				agtjson, err = json.Marshal(agt)
			}
			if err != nil {
				panic(err)
			}
			fmt.Printf("%s\n", agtjson)
		case "r":
			agt, err = getAgent(agtid, ctx)
			if err != nil {
				panic(err)
			}
			fmt.Println("Reload succeeded")
		case "":
			break
		default:
			fmt.Printf("Unknown order '%s'. You are in agent reader mode. Try `help`.\n", orders[0])
		}
		readline.AddHistory(input)
	}
exit:
	fmt.Printf("\n")
	return
}

func getAgent(agtid string, ctx Context) (agt mig.Agent, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("getAgent() -> %v", e)
		}
	}()
	targetURL := ctx.API.URL + "agent?agentid=" + agtid
	resource, err := getAPIResource(targetURL, ctx)
	if err != nil {
		panic(err)
	}
	if resource.Collection.Items[0].Data[0].Name != "agent" {
		panic("API returned something that is not an agent... something's wrong.")
	}
	agt, err = valueToAgent(resource.Collection.Items[0].Data[0].Value)
	if err != nil {
		panic(err)
	}
	return
}

func valueToAgent(v interface{}) (agt mig.Agent, err error) {
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

func printAgentLastActions(agtid string, limit int) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("printAgentLastActions() -> %v", e)
		}
	}()
	targetURL := fmt.Sprintf("%s/search?type=command&agentid=%s&limit=%d",
		ctx.API.URL, agtid, limit)
	resource, err := getAPIResource(targetURL, ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("-------  ID  ------- + --------    Action Name ------- + ----    Date    ---- +  -- Status --\n")
	for _, item := range resource.Collection.Items {
		for _, data := range item.Data {
			if data.Name != "command" {
				continue
			}
			cmd, err := valueToCommand(data.Value)
			if err != nil {
				panic(err)
			}
			name := cmd.Action.Name
			if len(name) < 30 {
				for i := len(name); i < 30; i++ {
					name += " "
				}
			}
			if len(name) > 30 {
				name = name[0:27] + "..."
			}
			fmt.Printf("%.0f     %s   %s    %s\n", cmd.ID, name,
				cmd.StartTime.Format(time.RFC3339), cmd.Status)
		}
	}
	return
}
