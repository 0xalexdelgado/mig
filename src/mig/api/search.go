// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]
package main

import (
	"fmt"
	"mig"
	migdb "mig/database"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/jvehent/cljs"
)

// search runs searches
func search(respWriter http.ResponseWriter, request *http.Request) {
	var err error
	opid := getOpID(request)
	loc := fmt.Sprintf("%s%s", ctx.Server.Host, request.URL.String())
	resource := cljs.New(loc)
	p := migdb.NewSearchParameters()
	defer func() {
		if e := recover(); e != nil {
			// on panic, log and return error to client, including the search parameters
			ctx.Channels.Log <- mig.Log{OpID: opid, Desc: fmt.Sprintf("%v", e)}.Err()
			resource.AddItem(cljs.Item{
				Href: loc,
				Data: []cljs.Data{{Name: "search parameters", Value: p}},
			})
			resource.SetError(cljs.Error{Code: fmt.Sprintf("%.0f", opid), Message: fmt.Sprintf("%v", e)})
			respond(500, resource, respWriter, request)
		}
		ctx.Channels.Log <- mig.Log{OpID: opid, Desc: "leaving search()"}.Debug()
	}()
	doFoundAnything := false
	timeLayout := time.RFC3339
	truere := regexp.MustCompile("(?i)^true$")
	falsere := regexp.MustCompile("(?i)^false$")
	for queryParams, _ := range request.URL.Query() {
		switch queryParams {
		case "actionname":
			p.ActionName = request.URL.Query()["actionname"][0]
		case "actionid":
			p.ActionID = request.URL.Query()["actionid"][0]
		case "after":
			p.After, err = time.Parse(timeLayout, request.URL.Query()["after"][0])
			if err != nil {
				panic("after date not in RFC3339 format")
			}
		case "agentid":
			p.AgentID = request.URL.Query()["agentid"][0]
		case "agentname":
			p.AgentName = request.URL.Query()["agentname"][0]
		case "before":
			p.Before, err = time.Parse(timeLayout, request.URL.Query()["before"][0])
			if err != nil {
				panic("before date not in RFC3339 format")
			}
		case "commandid":
			p.CommandID = request.URL.Query()["commandid"][0]
		case "foundanything":
			if truere.MatchString(request.URL.Query()["foundanything"][0]) {
				p.FoundAnything = true
			} else if falsere.MatchString(request.URL.Query()["foundanything"][0]) {
				p.FoundAnything = false
			} else {
				panic("foundanything parameter must be true or false")
			}
			doFoundAnything = true
		case "investigatorid":
			p.InvestigatorID = request.URL.Query()["investigatorid"][0]
		case "investigatorname":
			p.InvestigatorName = request.URL.Query()["investigatorname"][0]
		case "limit":
			p.Limit, err = strconv.ParseFloat(request.URL.Query()["limit"][0], 64)
			if err != nil {
				panic("invalid limit parameter")
			}
		case "report":
			switch request.URL.Query()["report"][0] {
			case "complianceitems":
				p.Report = request.URL.Query()["report"][0]
			default:
				panic("report not implemented")
			}
		case "status":
			p.Status = request.URL.Query()["status"][0]
		case "threatfamily":
			p.ThreatFamily = request.URL.Query()["threatfamily"][0]
		}
	}
	// run the search based on the type
	var results interface{}
	if _, ok := request.URL.Query()["type"]; ok {
		p.Type = request.URL.Query()["type"][0]
		switch p.Type {
		case "action":
			results, err = ctx.DB.SearchActions(p)
		case "agent":
			results, err = ctx.DB.SearchAgents(p)
		case "command":
			results, err = ctx.DB.SearchCommands(p, doFoundAnything)
		case "investigator":
			results, err = ctx.DB.SearchInvestigators(p)
		default:
			panic("search type is invalid")
		}
		if err != nil {
			panic(err)
		}
	} else {
		panic("search type is missing")
	}

	// prepare the output in the requested format
	switch p.Report {
	case "complianceitems":
		if p.Type != "command" {
			panic("compliance items not available for this type")
		}
		beforeStr := url.QueryEscape(p.Before.Format(time.RFC3339Nano))
		afterStr := url.QueryEscape(p.After.Format(time.RFC3339Nano))
		items, err := commandsToComplianceItems(results.([]mig.Command))
		if err != nil {
			panic(err)
		}
		for i, item := range items {
			err = resource.AddItem(cljs.Item{
				Href: fmt.Sprintf("%s%s/search?type=command?agentname=%s&commandid=%s&actionid=%s&threatfamily=compliance&report=complianceitems&after=%s&before=%s",
					ctx.Server.Host, ctx.Server.BaseRoute, item.Target,
					p.CommandID, p.ActionID, afterStr, beforeStr),
				Data: []cljs.Data{{Name: "compliance item", Value: item}},
			})
			if err != nil {
				panic(err)
			}
			if float64(i) > p.Limit {
				break
			}
		}
	default:
		switch p.Type {
		case "action":
			ctx.Channels.Log <- mig.Log{OpID: opid, Desc: fmt.Sprintf("returning search results with %d commands", len(results.([]mig.Action)))}
			for _, r := range results.([]mig.Action) {
				err = resource.AddItem(cljs.Item{
					Href: fmt.Sprintf("%s%s/action?actionid=%.0f",
						ctx.Server.Host, ctx.Server.BaseRoute, r.ID),
					Data: []cljs.Data{{Name: p.Type, Value: r}},
				})
				if err != nil {
					panic(err)
				}
			}
		case "agent":
			ctx.Channels.Log <- mig.Log{OpID: opid, Desc: fmt.Sprintf("returning search results with %d commands", len(results.([]mig.Agent)))}
			for _, r := range results.([]mig.Agent) {
				err = resource.AddItem(cljs.Item{
					Href: fmt.Sprintf("%s%s/agent?agentid=%.0f",
						ctx.Server.Host, ctx.Server.BaseRoute, r.ID),
					Data: []cljs.Data{{Name: p.Type, Value: r}},
				})
				if err != nil {
					panic(err)
				}
			}
		case "command":
			ctx.Channels.Log <- mig.Log{OpID: opid, Desc: fmt.Sprintf("returning search results with %d commands", len(results.([]mig.Command)))}
			for _, r := range results.([]mig.Command) {
				err = resource.AddItem(cljs.Item{
					Href: fmt.Sprintf("%s%s/command?commandid=%.0f",
						ctx.Server.Host, ctx.Server.BaseRoute, r.ID),
					Data: []cljs.Data{{Name: p.Type, Value: r}},
				})
				if err != nil {
					panic(err)
				}
			}
		case "investigator":
			ctx.Channels.Log <- mig.Log{OpID: opid, Desc: fmt.Sprintf("returning search results with %d investigators", len(results.([]mig.Investigator)))}
			for _, r := range results.([]mig.Investigator) {
				err = resource.AddItem(cljs.Item{
					Href: fmt.Sprintf("%s%s/investigator?investigatorid=%.0f",
						ctx.Server.Host, ctx.Server.BaseRoute, r.ID),
					Data: []cljs.Data{{Name: p.Type, Value: r}},
				})
				if err != nil {
					panic(err)
				}
			}
		}
	}
	// add search parameters at the end of the response
	err = resource.AddItem(cljs.Item{
		Href: loc,
		Data: []cljs.Data{{Name: "search parameters", Value: p}},
	})
	if err != nil {
		panic(err)
	}
	respond(200, resource, respWriter, request)
}
