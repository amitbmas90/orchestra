// task_state.go

package main

import (
	"os"
	"json"
)

type TaskState int

const (
	// Task is fresh and has never been sent to the client.  It can be rescheduled still.
	TASK_QUEUED		= TaskState(iota)
	// Task has been transmitted at least once
	TASK_PENDINGRESULT
	// Task has finished and we have received a result.
	TASK_FINISHED
)

func (ts TaskState) String() (strout string) {
	switch ts {
	case TASK_QUEUED:
		strout = "QUEUED"
	case TASK_PENDINGRESULT:
		strout = "PENDING"
	case TASK_FINISHED:
		strout = "FINISHED"
	default:
		strout = ""
	}
	return strout
}

func (ts TaskState) MarshalJSON() (out []byte, err os.Error) {
	strout := ts.String()
	if strout != "" {
		return json.Marshal(strout)
	}
	return nil, InvalidValueError
}

func (ts TaskState) UnmarshalJSON(in []byte) (err os.Error) {
	var statestr string
	err = json.Unmarshal(in, &statestr)
	if err != nil {
		return err
	}
	switch statestr {
	case "QUEUED":
		ts = TASK_QUEUED
	case "PENDING":
		ts = TASK_PENDINGRESULT
	case "FINISHED":
		ts = TASK_FINISHED
	default:
		return InvalidValueError
	}
	return nil
}

func (ts TaskState) Finished() bool {
	if ts == TASK_FINISHED {
		return true
	}
	return false
}