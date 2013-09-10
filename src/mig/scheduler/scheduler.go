
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/howeyc/fsnotify"
	"github.com/streadway/amqp"
	"io/ioutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"log"
	//"math/rand"
	"mig"
	"os"
	"strings"
	"time"
)

var NEWACTIONDIR string		= "/var/cache/mig/actions/new"
var LAUNCHCMDDIR string		= "/var/cache/mig/commands/ready"
var INFLIGHTCMDDIR string	= "/var/cache/mig/commands/inflight"
var DONECMDDIR string		= "/var/cache/mig/commands/done"
var DONEACTIONDIR string	= "/var/cache/mig/actions/done"
var AMQPBROKER string		= "amqp://guest:guest@172.21.1.1:5672/"
var MONGOURI string		= "172.21.2.143"

// pullAction parses a new action from the input dir and prepares it for launch
func pullAction(actionNewChan <-chan string, mgoRegCol *mgo.Collection) error {
	for actionPath := range actionNewChan{
		rawAction, err := ioutil.ReadFile(actionPath)
		if err != nil {
			log.Fatal("pullAction - ReadFile(): ", err)
		}
		var action mig.Action
		err = json.Unmarshal(rawAction, &action)
		if err != nil {
			log.Fatal("pullAction - json.Unmarshal:", err)
		}
		err = validateActionSyntax(action)
		if err != nil {
			log.Println("pullAction - validateActionSyntax(): ", err)
			log.Println("pullAction - Deleting invalid action: ", actionPath)
			// action with invalid syntax are deleted
			os.Remove(actionPath)
			continue
		}
		log.Println("pullAction: new action received:",
				"Name:", action.Name,
				", Target:", action.Target,
				", Check:", action.Check,
				", RunDate:", action.RunDate,
				", Expiration:", action.Expiration,
				", Arguments:", action.Arguments)
		// get the list of targets from the register
		targets := []mig.Register{}
		iter := mgoRegCol.Find(bson.M{"os": action.Target}).Iter()
		err = iter.All(&targets)
		if err != nil {
			log.Fatal("pullAction - iter.All():", err)
		}
		for _, target := range targets {
			log.Println("pullAction: scheduling action", action.Name, "on target", target.Name)
			cmd := mig.Command{
				AgentName: target.Name,
				AgentQueueLoc: target.QueueLoc,
				Action: action,
			}
			jsonCmd, err := json.Marshal(cmd)
			if err != nil {
				log.Fatal("pullAction - json.Marshal():", err)
			}
			cmdPath := LAUNCHCMDDIR + "/" + target.QueueLoc + ".json"
			err = ioutil.WriteFile(cmdPath, jsonCmd, 0640)
		}
		os.Remove(actionPath)
	}
	return nil
}

func validateActionSyntax(action mig.Action) error {
	if action.Name == "" {
		return errors.New("Action.Name is empty. Expecting string.")
	}
	if action.Target == "" {
		return errors.New("Action.Target is empty. Expecting string.")
	}
	if action.Check == "" {
		return errors.New("Action.Check is empty. Expecting string.")
	}
	if action.RunDate == "" {
		return errors.New("Action.RunDate is empty. Expecting string.")
	}
	if action.Expiration == "" {
		return errors.New("Action.Expiration is empty. Expecting string.")
	}
	if action.Arguments == nil {
		return errors.New("Action.Arguments is nil. Expecting string.")
	}
	return nil
}

/*
// receive a raw action and check its syntax
func parseAction() error {
}

// expand the action into individual commands and store in commands dirs
func prepareActionLaunch() error {
}
*/
// send actions from command dir to agent via AMQP
func launchCommand(cmdLaunchChan <-chan string) error {
	for cmd := range cmdLaunchChan{
		log.Println(cmd)
	}
	return nil
}

// keep track of running commands, requeue expired onces
func updateCommandStatus(cmdInFlightChan <-chan string) error {
	for cmd := range cmdInFlightChan{
		log.Println(cmd)
	}
	return nil
}

// keep track of running actions
//func updateActionStatus() error {
//}

// store the result of a command and mark it as completed/failed
func terminateCommand(cmdDoneChan <-chan string) error {
	for cmd := range cmdDoneChan{
		log.Println(cmd)
	}
	return nil
}

// store the result of an action and mark it as completed
func terminateAction(actionDoneChan <-chan string) error {
	for act := range actionDoneChan{
		log.Println(act)
	}
	return nil
}
/*
func sendActions(c *amqp.Channel) error {
	r := rand.New(rand.NewSource(65537))
	for {
		action := mig.Action{
			ActionID: fmt.Sprintf("TestFilechecker%d", r.Intn(1000000000)),
			Target:	  "all",
			Check:    "filechecker",
			Command:  "/usr/bin/vim:sha256=a2fed99838d60d9dc920c5adc61800a48f116c230a76c5f2586487ba09c72d42",
		}
		actionJson, err := json.Marshal(action)
		if err != nil {
			log.Fatal("sendActions - json.Marshal:", err)
		}
		msg := amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			ContentType:  "text/plain",
			Body:         []byte(actionJson),
		}
		log.Printf("Creating action: '%s'", actionJson)
		err = c.Publish("mig",		// exchange name
				"mig.all",	// exchange key
				true,		// is mandatory
				false,		// is immediate
				msg)		// AMQP message
		if err != nil {
			log.Fatal("sendActions - Publish():", err)
		}
		time.Sleep( 60 * time.Second)
	}
	return nil
}
*/

func listenToAgent(agentChan <-chan amqp.Delivery, c *amqp.Channel) error {
	for m := range agentChan {
		log.Printf("listenToAgent: queue '%s' received '%s'",
			m.RoutingKey, m.Body)
		// Ack this message only
		err := m.Ack(true)
		if err != nil {
			log.Fatal("listenToAgent - Ack():", err)
		}
	}
	return nil
}

func getRegistrations(registration <-chan amqp.Delivery, c *amqp.Channel, mgoRegCol *mgo.Collection) error {
	var reg mig.Register
	for r := range registration{
		err := json.Unmarshal(r.Body, &reg)
		if err != nil {
			log.Fatal("getRegistration - json.Unmarshal:", err)
		}
		log.Println("getRegistrations: Agent Name:", reg.Name, ";",
			    "Agent OS:", reg.OS, "; Agent ID:", reg.QueueLoc)

		//create a queue for agt message
		queue := fmt.Sprintf("mig.scheduler.%s", reg.QueueLoc)
		_, err = c.QueueDeclare(queue, true, false, false, false, nil)
		if err != nil {
			log.Fatalf("QueueDeclare: %v", err)
		}
		err = c.QueueBind(queue, queue,	"mig", false, nil)
		if err != nil {
			log.Fatalf("QueueBind: %v", err)
		}
		agentChan, err := c.Consume(queue, "", false, false, false, false, nil)
		go listenToAgent(agentChan, c)

		//save registration in database
		reg.LastRegistrationTime =  time.Now()
		reg.LastHeartbeatTime = time.Now()

		// try to find an existing entry to update
		log.Println("getRegistrations: Updating registration info for agent", reg.Name)
		err = mgoRegCol.Update(	bson.M{"name": reg.Name, "os": reg.OS, "queueloc": reg.QueueLoc},
					bson.M{"lastregistrationtime": time.Now(), "lastheartbeattime": time.Now()})
		if err != nil {
			log.Println("getRegistrations: Registration update failed, creating new entry for agent", reg.Name)
			reg.FirstRegistrationTime = time.Now()
			err = mgoRegCol.Insert(reg)
		}
		err = r.Ack(true)
	}
	return nil
}

func main() {
	termChan	:= make(chan bool)
	actionNewChan	:= make(chan string)
	cmdLaunchChan	:= make(chan string)
	cmdInFlightChan	:= make(chan string)
	cmdDoneChan	:= make(chan string)
	actionDoneChan	:= make(chan string)

	// Setup connection to MongoDB backend database
	mgofd, err := mgo.Dial(MONGOURI)
	if err != nil {
		log.Fatal("Main: MongoDB connection error: ", err)
	}
	defer mgofd.Close()
	mgofd.SetSafe(&mgo.Safe{})	// make safe writes only
	mgoRegCol := mgofd.DB("mig").C("registrations")
	log.Println("Main: MongoDB connection successfull. URI=", MONGOURI)

	// Watch the data directories for new files
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("fsnotify.NewWatcher(): ", err)
	}
	go func() {
		for {
			select {
			case ev := <-watcher.Event :
				log.Println("event: ", ev)
				if strings.HasPrefix(ev.Name, NEWACTIONDIR) {
					log.Println("Action received:", ev)
					actionNewChan <- ev.Name
				} else if strings.HasPrefix(ev.Name, LAUNCHCMDDIR) {
					log.Println("Command ready:", ev)
					cmdLaunchChan <- ev.Name
				} else if strings.HasPrefix(ev.Name, INFLIGHTCMDDIR) {
					log.Println("Command in flight:", ev)
					cmdInFlightChan <- ev.Name
				} else if strings.HasPrefix(ev.Name, DONECMDDIR) {
					log.Println("Command done:", ev)
					cmdDoneChan <- ev.Name
				} else if strings.HasPrefix(ev.Name, DONEACTIONDIR) {
					log.Println("Action done:", ev)
					actionDoneChan <- ev.Name
				}
			case err := <-watcher.Error:
				log.Println("error: ", err)
			}
		}
	}()
	err = watcher.WatchFlags(NEWACTIONDIR, fsnotify.FSN_CREATE)
	if err != nil {
		log.Fatal("watcher.Watch(): ",err)
	}
	log.Println("Main: Initializer watcher on", NEWACTIONDIR)
	err = watcher.WatchFlags(LAUNCHCMDDIR, fsnotify.FSN_CREATE)
	if err != nil {
		log.Fatal("watcher.Watch(): ",err)
	}
	log.Println("Main: Initializer watcher on", LAUNCHCMDDIR)
	err = watcher.WatchFlags(INFLIGHTCMDDIR, fsnotify.FSN_CREATE)
	if err != nil {
		log.Fatal("watcher.Watch(): ",err)
	}
	log.Println("Main: Initializer watcher on", INFLIGHTCMDDIR)
	err = watcher.WatchFlags(DONECMDDIR, fsnotify.FSN_CREATE)
	if err != nil {
		log.Fatal("watcher.Watch(): ",err)
	}
	log.Println("Main: Initializer watcher on", DONECMDDIR)
	err = watcher.WatchFlags(DONEACTIONDIR, fsnotify.FSN_CREATE)
	if err != nil {
		log.Fatal("watcher.Watch(): ",err)
	}
	log.Println("Main: Initializer watcher on", DONEACTIONDIR)
	// launch the routines that process action & commands
	go pullAction(actionNewChan, mgoRegCol)
	log.Println("Main: pullAction goroutine started")
	go launchCommand(cmdLaunchChan)
	log.Println("Main: launchCommand goroutine started")
	go updateCommandStatus(cmdInFlightChan)
	log.Println("Main: updateCommandStatus gorouting started")
	go terminateCommand(cmdDoneChan)
	log.Println("Main: terminateCommand goroutine started")
	go terminateAction(actionDoneChan)
	log.Println("Main: terminateAction goroutine started")

	// Setup the AMQP connections and get ready to recv/send messages
	conn, err := amqp.Dial(AMQPBROKER)
	if err != nil {
		log.Fatalf("amqp.Dial(): %v", err)
	}
	defer conn.Close()
	log.Println("Main: AMQP connection succeeded. Broker=", AMQPBROKER)
	c, err := conn.Channel()
	if err != nil {
		log.Fatalf("Channel(): %v", err)
	}
	// main exchange for all publications
	err = c.ExchangeDeclare("mig", "topic", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("ExchangeDeclare: %v", err)
	}
	// agent registrations
	_, err = c.QueueDeclare("mig.register", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("QueueDeclare: %v", err)
	}
	err = c.QueueBind("mig.register", "mig.register", "mig", false,	nil)
	if err != nil {
		log.Fatalf("QueueBind: %v", err)
	}
	err = c.Qos(1,0, false)
	if err != nil {
		log.Fatalf("ChannelQoS: %v", err)
	}
	regChan, err := c.Consume("mig.register", "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("ChannelConsume: %v", err)
	}
	log.Println("Main: Registration channel initialized")
	// launch the routine that handles registrations
	go getRegistrations(regChan, c, mgoRegCol)
	log.Println("Main: getRegistrations goroutine started")

	log.Println("Main: Initialization completed successfully")
	// won't exit until this chan received something
	<-termChan
}
