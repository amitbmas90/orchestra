// resp_state.go

package main

import (
	"encoding/json"
)

type ResponseState int

const (
	// Response states
	RESP_PENDING			= ResponseState(iota)	// internal state, not wire.
	RESP_RUNNING
	RESP_FINISHED
	RESP_FAILED
	RESP_FAILED_UNKNOWN_SCORE
	RESP_FAILED_HOST_ERROR
	RESP_FAILED_UNKNOWN
)

func (rs ResponseState) String() (strout string) {
	switch rs {
	case RESP_RUNNING:
		return "PENDING"
	case RESP_FINISHED:
		return "OK"
	case RESP_FAILED:
		return "FAIL"
	case RESP_FAILED_UNKNOWN_SCORE:
		return "UNK_SCORE"
	case RESP_FAILED_HOST_ERROR:
		return "HOST_ERROR"
	case RESP_FAILED_UNKNOWN:
		return "UNKNOWN_FAILURE"
	}
	return ""
}

func (rs ResponseState) MarshalJSON() (out []byte, err error) {
	strout := rs.String()
	if strout != "" {
		return json.Marshal(strout)
	}
	return nil, InvalidValueError
}

func (rs ResponseState) UnmarshalJSON(in []byte) (err error) {
	var statestr string
	err = json.Unmarshal(in, &statestr)
	if err != nil {
		return err
	}
	switch statestr {
	case "PENDING":
		rs = RESP_PENDING
	case "OK":
		rs = RESP_FINISHED
	case "FAIL":
		rs = RESP_FAILED
	case "UNK_SCORE":
		rs = RESP_FAILED_UNKNOWN_SCORE
	case "HOST_ERROR":
		rs = RESP_FAILED_HOST_ERROR
	case "UNKNOWN_FAILURE":
		rs = RESP_FAILED_UNKNOWN
	default:
		return InvalidValueError
	}
	return nil
}

func (rs ResponseState) Finished() bool {
	switch rs {
	case RESP_FINISHED:
		fallthrough
	case RESP_FAILED:
		fallthrough
	case RESP_FAILED_UNKNOWN_SCORE:
		fallthrough
	case RESP_FAILED_HOST_ERROR:
		fallthrough
	case RESP_FAILED_UNKNOWN:
		return true
	}
	return false
}

// true if the response says the task failed.  false otherwise.
func (rs ResponseState) Failed() bool {
	switch rs {
	case RESP_RUNNING:
		fallthrough
	case RESP_FINISHED:
		return false
	}
	return true
}

// true if the task can be tried.
// precond:  DidFail is true, job is a ONE_OF job.
// must return false otherwise.
func (rs ResponseState) CanRetry() bool {
	switch rs {
	case RESP_FAILED_UNKNOWN_SCORE:
		fallthrough
	case RESP_FAILED_HOST_ERROR:
		return true
	}
	return false
}