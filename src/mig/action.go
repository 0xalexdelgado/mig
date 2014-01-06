package mig

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"math/rand"
	"time"
)

type Action struct {
	ID uint64
	Name, Target, Check string
	ScheduledDate, ExpirationDate time.Time
	Arguments interface{}
}

type ExtendedAction struct{
	Action Action
	Status string
	StartTime, FinishTime, LastUpdateTime time.Time
	CommandIDs []uint64
	CmdCompleted, CmdCancelled, CmdTimedOut int
	Signature []string
	SignatureDate time.Time
}

// FromFile reads an action from a local file on the file system
// and returns a mig.ExtendedAction structure
func ActionFromFile(path string) (ea ExtendedAction, err error){
	defer func() {
		if e := recover(); e != nil {
			reason := fmt.Sprintf("mig.Action.FromFile(): %v", e)
			err = errors.New(reason)
			return
		}
	}()
	// parse the json of the action into a mig.ExtendedAction
	fd, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(fd, &ea.Action)
	if err != nil {
		panic(err)
	}

	// generate an action id
	ea.Action.ID = GenID()

	// syntax checking
	err = checkAction(ea.Action)
	if err != nil {
		panic(err)
	}

	// Populate the Extended attributes of the action
	ea.StartTime = time.Now().UTC()

	return
}

// genID returns an ID composed of a unix timestamp and a random CRC32
func GenID() uint64 {
	h := crc32.NewIEEE()
	t := time.Now().UTC().Format(time.RFC3339Nano)
	r := rand.New(rand.NewSource(65537))
	rand := string(r.Intn(1000000000))
	h.Write([]byte(t + rand))
	// concatenate timestamp and hash into 64 bits ID
	// id = <32 bits unix ts><32 bits CRC hash>
	id := uint64(time.Now().Unix())
	id = id << 32
	id += uint64(h.Sum32())
	return id
}


// checkAction verifies that the Action received contained all the
// necessary fields, and returns an error when it doesn't.
func checkAction(action Action) error {
	if action.Name == "" {
		return errors.New("Action.Name is empty. Expecting string.")
	}
	if action.Target == "" {
		return errors.New("Action.Target is empty. Expecting string.")
	}
	if action.Check == "" {
		return errors.New("Action.Check is empty. Expecting string.")
	}
	if action.ScheduledDate.String() == "" {
		return errors.New("Action.RunDate is empty. Expecting string.")
	}
	if action.ExpirationDate.String() == "" {
		return errors.New("Action.Expiration is empty. Expecting string.")
	}
	if action.Arguments == nil {
		return errors.New("Action.Arguments is nil. Expecting string.")
	}
	return nil
}

