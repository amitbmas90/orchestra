// job_state.go

package main

import (
	"encoding/json"
)

type JobState int

const (
	JOB_STATE_INVALID	= JobState(iota)
	// Job is pending resolution
	JOB_PENDING
	// Job has completed and has no failures.
	JOB_SUCCESSFUL
	// Job has completed and has mixed results.
	JOB_FAILED_PARTIAL
	// Job has completed and has completely failed.
	JOB_FAILED
)

func (js JobState) Finished() bool {
	if js == JOB_PENDING {
		return false
	}
	return true
}

func (js JobState) String() (strout string) {
	switch js {
	case JOB_PENDING:
		strout = "PENDING"
	case JOB_SUCCESSFUL:
		strout = "OK"
	case JOB_FAILED:
		strout = "FAIL"
	case JOB_FAILED_PARTIAL:
		strout = "PARTIAL_FAIL"
	default:
		strout = ""
	}
	return strout

}

func (js JobState) MarshalJSON() (out []byte, err error) {
	strout := js.String()
	if strout != "" {
		return json.Marshal(strout)
	}
	return nil, InvalidValueError
}

func (js *JobState) UnmarshalJSON(in []byte) (err error) {
	var statestr string
	err = json.Unmarshal(in, &statestr)
	if err != nil {
		return err
	}
	switch statestr {
	case "PENDING":
		*js = JOB_PENDING
	case "OK":
		*js = JOB_SUCCESSFUL
	case "FAIL":
		*js = JOB_FAILED
	case "PARTIAL_FAIL":
		*js = JOB_FAILED_PARTIAL
	default:
		return InvalidValueError
	}
	return nil
}

