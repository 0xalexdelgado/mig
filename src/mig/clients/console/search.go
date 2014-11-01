// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]
package main

import (
	"fmt"
	"mig"
	"regexp"
	"strings"
	"time"

	"github.com/jvehent/cljs"
)

type searchParameters struct {
	sType   string
	query   string
	version string
}

// search runs a search for actions, commands or agents
func search(input string, ctx Context) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("search() -> %v", e)
		}
	}()
	orders := strings.Split(input, " ")
	if len(orders) < 2 {
		orders = append(orders, "help")
	}
	sType := ""
	switch orders[1] {
	case "action", "agent", "command", "investigator":
		sType = orders[1]
	case "", "help":
		fmt.Printf(`usage: search <action|agent|command|investigator> where <parameters> [<and|or> <parameters>]
The following search parameters are available:
`)
		return nil
	default:
		return fmt.Errorf("Invalid search '%s'. Try `search help`.\n", input)
	}
	sp, err := parseSearchQuery(orders)
	if err != nil {
		panic(err)
	}
	items, err := runSearchQuery(sp, ctx)
	if err != nil {
		panic(err)
	}
	switch sType {
	case "agent":
		agents, err := filterAgentItems(sp, items, ctx)
		if err != nil {
			panic(err)
		}
		fmt.Println("----    ID      ---- + ----         Name         ---- + -- Last Heartbeat --")
		for _, agt := range agents {
			name := agt.Name
			if useShortNames {
				name = shorten(name)
			}
			if len(name) < 30 {
				for i := len(name); i < 30; i++ {
					name += " "
				}
			}
			if len(name) > 30 {
				name = name[0:27] + "..."
			}
			fmt.Printf("%20.0f   %s   %s\n", agt.ID, name[0:30], agt.HeartBeatTS.Format(time.RFC3339))
		}
	case "action", "command":
		fmt.Println("----    ID      ---- + ----         Name         ---- + --- Last Updated ---")
		for _, item := range items {
			for _, data := range item.Data {
				if data.Name != sType {
					continue
				}
				switch data.Name {
				case "action":
					idstr, name, datestr, _, err := actionPrintShort(data.Value)
					if err != nil {
						panic(err)
					}
					fmt.Printf("%s   %s   %s\n", idstr, name, datestr)
				case "command":
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
					fmt.Printf("%20.0f   %s   %s\n", cmd.ID, name, cmd.FinishTime.Format(time.RFC3339))
				}
			}
		}
	case "investigator":
		fmt.Println("- ID - + ----         Name         ---- + --- Status ---")
		for _, item := range items {
			for _, data := range item.Data {
				if data.Name != sType {
					continue
				}
				switch data.Name {
				case "investigator":
					inv, err := valueToInvestigator(data.Value)
					if err != nil {
						panic(err)
					}
					name := inv.Name
					if len(name) < 30 {
						for i := len(name); i < 30; i++ {
							name += " "
						}
					}
					if len(name) > 30 {
						name = name[0:27] + "..."
					}
					fmt.Printf("%6.0f   %s   %s\n", inv.ID, name, inv.Status)
				}
			}
		}
	}
	return
}

// parseSearchQuery transforms a search string into an API query
func parseSearchQuery(orders []string) (sp searchParameters, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("parseSearchQuery() -> %v", e)
		}
	}()
	sType := orders[1]
	query := "search?type=" + sType
	if len(orders) < 4 {
		panic("Invalid search syntax. try `search help`.")
	}
	if orders[2] != "where" {
		panic(fmt.Sprintf("Expected keyword 'where' after search type. Got '%s'", orders[2]))
	}
	for _, order := range orders[3:len(orders)] {
		if order == "and" || order == "or" {
			continue
		}
		params := strings.Split(order, "=")
		if len(params) != 2 {
			panic(fmt.Sprintf("Invalid `key=value` for in parameter '%s'", order))
		}
		key := params[0]
		// if the string contains % characters, used in postgres's pattern matching,
		// escape them properly
		value := strings.Replace(params[1], "%", "%25", -1)
		// wildcards are converted to postgres's % pattern matching
		value = strings.Replace(value, "*", "%25", -1)
		switch key {
		case "and", "or":
			continue
		case "agentname":
			query += "&agentname=" + value
		case "after":
			query += "&after=" + value
		case "before":
			query += "&before=" + value
		case "id":
			panic("If you already know the ID, don't use the search. Use (action|command|agent) <id> directly")
		case "actionid":
			query += "&actionid=" + value
		case "commandid":
			query += "&commandid=" + value
		case "agentid":
			query += "&agentid=" + value
		case "name":
			switch sType {
			case "action", "command":
				query += "&actionname=" + value
			case "agent":
				query += "&agentname=" + value
			}
		case "status":
			switch sType {
			case "action":
				panic("'status' is not a valid action search parameter")
			case "command", "agent":
				query += "&status=" + value
			}
		case "limit":
			query += "&limit=" + value
		case "version":
			if sType != "agent" {
				panic("'version' is only valid when searching for agents")
			}
			sp.version = value
		default:
			panic(fmt.Sprintf("Unknown search key '%s'", key))
		}
	}
	sp.sType = sType
	sp.query = query
	return
}

// runSearchQuery executes a search string against the API
func runSearchQuery(sp searchParameters, ctx Context) (items []cljs.Item, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("runSearchQuery() -> %v", e)
		}
	}()
	fmt.Println("Search query:", sp.query)
	targetURL := ctx.API.URL + sp.query
	resource, err := getAPIResource(targetURL, ctx)
	if err != nil {
		panic(err)
	}
	items = resource.Collection.Items
	return
}

func filterAgentItems(sp searchParameters, items []cljs.Item, ctx Context) (agents []mig.Agent, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("filterAgentItems() -> %v", e)
		}
	}()
	for _, item := range items {
		for _, data := range item.Data {
			if data.Name != sp.sType {
				continue
			}
			switch sp.sType {
			case "agent":
				agt, err := valueToAgent(data.Value)
				if err != nil {
					panic(err)
				}
				if sp.version != "" {
					tests := strings.Split(sp.version, "%")
					for _, test := range tests {
						if !strings.Contains(agt.Version, test) {
							// this agent doesn't have the version we are looking for, skip it
							goto skip
						}
					}
				}
				agents = append(agents, agt)
			}
		skip:
		}
	}
	return
}

// filterString matches an input string against a filter that's an array of string in the form
// ['|', 'grep', 'something', '|', 'grep', '-v', 'notsomething']
func filterString(input string, filter []string) (output string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("filterString() -> %v", e)
		}
	}()
	const (
		modeNull = 1 << iota
		modePipe
		modeGrep
		modeInverseGrep
		modeConsumed
	)
	mode := modeNull
	for _, comp := range filter {
		switch comp {
		case "|":
			if mode != modeNull {
				panic("Invalid pipe placement")
			}
			mode = modePipe
			continue
		case "grep":
			if mode != modePipe {
				panic("grep must be preceded by a pipe")
			}
			mode = modeGrep
		case "-v":
			if mode != modeGrep {
				panic("-v is an option of grep, but grep is missing")
			}
			mode = modeInverseGrep
		default:
			if mode == modeNull {
				panic("unknown filter mode")
			} else if (mode == modeGrep) || (mode == modeInverseGrep) {
				re, err := regexp.CompilePOSIX(comp)
				if err != nil {
					panic(err)
				}
				if re.MatchString(input) {
					// the string matches, but we want inverse grep
					if mode == modeInverseGrep {
						return "", err
					}
				} else {
					// the string doesn't match, and we want grep
					if mode == modeGrep {
						return "", err
					}
				}
			} else {
				panic("unrecognized filter syntax")
			}
			// reset the mode
			mode = modeNull
		}
	}
	output = input
	return
}
