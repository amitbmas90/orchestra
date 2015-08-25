// task_state.go

package main

import (
	"json"
)

type TaskState int

const (
	TASK_INVALID		= TaskState(iota)
	// Task is fresh and has never been sent to the client.  It can be rescheduled still.
	TASK_QUEUED
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

func (ts TaskState) MarshalJSON() (out []byte, err error) {
	strout := ts.String()
	if strout != "" {
		return json.Marshal(strout)
	}
	return nil, InvalidValueError
}

func (ts *TaskState) UnmarshalJSON(in []byte) (err error) {
	var statestr string
	err = json.Unmarshal(in, &statestr)
	if err != nil {
		return err
	}
	switch statestr {
	case "QUEUED":
		*ts = TASK_QUEUED
	case "PENDING":
		*ts = TASK_PENDINGRESULT
	case "FINISHED":
		*ts = TASK_FINISHED
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