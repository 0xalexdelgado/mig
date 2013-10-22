
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/streadway/amqp"
	"log"
	"mig"
	"mig/modules/filechecker"
	"os/exec"
	"os"
	"runtime"
	"time"
)

var AMQPBROKER string = "amqp://guest:guest@172.21.1.1:5672/"
var HEARTBEATFREQ string = "600s"

func getCommands(messages <-chan amqp.Delivery, actions chan []byte, terminate chan bool) error {
	// range waits on the channel and returns all incoming messages
	// range will exit when the channel closes
	for m := range messages {
		log.Printf("getCommands: received '%s'", m.Body)
		// Ack this message only
		err := m.Ack(true)
		if err != nil { panic(err) }
		actions <- m.Body
		log.Printf("getCommands: queued in pos. %d", len(actions))
	}
	terminate <- true
	return nil
}

func parseCommands(commands <-chan []byte, fCommandChan chan mig.Command, terminate chan bool) error {
	var cmd mig.Command
	for a := range commands {
		err := json.Unmarshal(a, &cmd)
		if err != nil {
			log.Fatal("parseCommand - json.Unmarshal:", err)
		}
		log.Printf("ParseCommand: Check '%s' Arguments '%s'",
			   cmd.Action.Check, cmd.Action.Arguments)
		switch cmd.Action.Check{
		case "filechecker":
			fCommandChan <- cmd
			log.Println("parseCommands: queued into filechecker",
				    "in pos.", len(fCommandChan))
		}
	}
	terminate <- true
	return nil
}

func runFilechecker(fCommandChan <-chan mig.Command, alertChan chan mig.Alert, resultChan chan mig.Command, terminate chan bool) error {
	for migCmd := range fCommandChan {
		log.Printf("RunFilechecker: running with args '%s'", migCmd.Action.Arguments)
		var cmdArg string
		for _, arg := range migCmd.Action.Arguments {
			cmdArg += arg
		}
		runCmd := exec.Command("./filechecker", cmdArg)
		cmdout, err := runCmd.StdoutPipe()
		if err != nil {
			log.Fatal(err)
		}
		st := time.Now()
		err = runCmd.Start()
		if err != nil {
			log.Fatal(err)
		}
		results := make(map[string] mig.FileCheckerResult)
		err = json.NewDecoder(cmdout).Decode(&results)
		if err != nil {
			log.Fatal(err)
		}
		cmdDone := make(chan error)
		go func() {
			cmdDone <-runCmd.Wait()
		}()
		select {
		// kill the process when timeout expires
		case <-time.After(30 * time.Second):
			if err := runCmd.Process.Kill(); err != nil {
				log.Fatal("failed to kill:", err)
			}
			log.Fatal("runFileChecker: command '%s' timed out", migCmd)
		// exit normally
		case err := <-cmdDone:
			if err != nil {
				log.Fatal(err)
			}
		}
		for _, r := range results {
			log.Println("runFileChecker: command", migCmd,"tested",
				    r.TestedFiles, "files in", time.Now().Sub(st))
			if r.ResultCount > 0 {
				for _, f := range r.Files {
					alertChan <- mig.Alert{
						Arguments: migCmd.Action.Arguments,
						Item: f,
					}
				}
			}
			migCmd.FCResults = append(migCmd.FCResults, r)
		}
		resultChan <- migCmd
	}
	terminate <- true
	return nil
}

func raiseAlerts(alertChan chan mig.Alert, terminate chan bool) error {
	for a := range alertChan {
		log.Printf("raiseAlerts: IOC '%s' positive match on '%s'",
			   a.Arguments, a.Item)
	}
	return nil
}

func sendResults(c *amqp.Channel, agtQueueLoc string, resultChan <-chan mig.Command, terminate chan bool) error {
	rKey := fmt.Sprintf("mig.agents.%s", agtQueueLoc)
	for r := range resultChan {
		r.AgentQueueLoc = agtQueueLoc
		body, err := json.Marshal(r)
		if err != nil {
			log.Fatalf("sendResults - json.Marshal: %v", err)
		}
		msgXchange(c, "mig", rKey, body)
	}
	return nil
}

func keepAliveAgent(c *amqp.Channel, regMsg mig.KeepAlive) error {
	sleepTime, err := time.ParseDuration(HEARTBEATFREQ)
	if err != nil {
		log.Fatal("sendHeartbeat - time.ParseDuration():", err)
	}
	for {
		body, err := json.Marshal(regMsg)
		if err != nil {
			log.Fatal("sendHeartbeat - json.Marshal:", err)
		}
		msgXchange(c, "mig", "mig.keepalive", body)
		time.Sleep(sleepTime)
	}
	return nil
}

func msgXchange(c *amqp.Channel, excName, routingKey string, body []byte) error {
	msg := amqp.Publishing{
	    DeliveryMode: amqp.Persistent,
	    Timestamp:    time.Now(),
	    ContentType:  "text/plain",
	    Body:         []byte(body),
	}
	err := c.Publish(excName,
			routingKey,
			true,	// is mandatory
			false,	// is immediate
			msg)	// AMQP message
	if err != nil {
		log.Fatalf("msgXchange - ChannelPublish: %v", err)
	}
	log.Printf("msgXchange: published '%s'\n", msg.Body)
	return nil
}

func main() {
	// parse command line argument
	// -m selects the mode {agent, filechecker, ...}
	var mode = flag.String("m", "agent", "module to run (eg. agent, filechecker)")
	flag.Parse()
	switch *mode {
	case "filechecker":
		fmt.Printf(filechecker.Run(flag.Args()))
		os.Exit(0)
	case "agent":
		fmt.Printf("Launching MIG Agent")
	}

	// termChan is used to exit the program
	termChan	:= make(chan bool)
	actionsChan	:= make(chan []byte, 10)
	fCommandChan	:= make(chan mig.Command, 10)
	alertChan	:= make(chan mig.Alert, 10)
	resultChan	:= make(chan mig.Command, 10)
	hostname, err	:= os.Hostname()
	if err != nil {
		log.Fatalf("os.Hostname(): %v", err)
	}
	regMsg := mig.KeepAlive{
		Name: hostname,
		OS: runtime.GOOS,
		QueueLoc: fmt.Sprintf("%s.%s", runtime.GOOS, hostname),
		LastKeepAlive: time.Now(),
	}
	agentQueue := fmt.Sprintf("mig.agt.%s", regMsg.QueueLoc)
	bindings := []mig.Binding{
		mig.Binding{agentQueue, agentQueue},
		mig.Binding{agentQueue, "mig.all"},
	}

	log.Println("MIG agent starting on", hostname)

	// Connects opens an AMQP connection from the credentials in the URL.
	conn, err := amqp.Dial(AMQPBROKER)
	if err != nil {
		log.Fatalf("amqp.Dial(): %v", err)
	}
	defer conn.Close()
	c, err := conn.Channel()
	if err != nil {
		log.Fatalf("conn.Channel(): %v", err)
	}
	for _, b := range bindings {
		_, err = c.QueueDeclare(b.Queue,	// Queue name
					true,		// is durable
					false,		// is autoDelete
					false,		// is exclusive
					false,		// is noWait
					nil)		// AMQP args
		if err != nil {
			log.Fatalf("QueueDeclare: %v", err)
		}
		err = c.QueueBind(b.Queue,	// Queue name
				b.Key,		// Routing key name
				"mig",		// Exchange name
				false,		// is noWait
				nil)		// AMQP args
		if err != nil {
			log.Fatalf("QueueBind: %v", err)
		}
	}

	// Limit the number of message the channel will receive
	err = c.Qos(2,		// prefetch count (in # of msg)
		    0,		// prefetch size (in bytes)
		    false)	// is global
	if err != nil {
		log.Fatalf("ChannelQoS: %v", err)
	}
	for _, b := range bindings {
		msgChan, err := c.Consume(b.Queue, // queue name
					"",	// some tag
					false,	// is autoAck
					false,	// is exclusive
					false,	// is noLocal
					false,	// is noWait
					nil)	// AMQP args
		if err != nil {
			log.Fatalf("ChannelConsume: %v", err)
		}
		go getCommands(msgChan, actionsChan, termChan)
	}
	go parseCommands(actionsChan, fCommandChan, termChan)
	go runFilechecker(fCommandChan, alertChan, resultChan, termChan)
	go raiseAlerts(alertChan, termChan)
	go sendResults(c, regMsg.QueueLoc, resultChan, termChan)

	// All set, ready to keepAlive
	go keepAliveAgent(c, regMsg)

	// block until terminate chan is called
	<-termChan
}
