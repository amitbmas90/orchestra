// job_scope.go

package main

import (
	"os"
	"encoding/json"
)

const (
	SCOPE_INVALID		= JobScope(iota)
	SCOPE_ONEOF
	SCOPE_ALLOF
)

type JobScope int

func (js JobScope) String() (strout string) {
	switch js {
	case SCOPE_ONEOF:
		strout = "one"
	case SCOPE_ALLOF:
		strout = "all"
	default:
		strout = ""
	}
	return strout
}

func (js JobScope) MarshalJSON() (out []byte, err os.Error) {
	strout := js.String()
	if strout != "" {
		return json.Marshal(strout)
	}
	return nil, InvalidValueError
}

func (js *JobScope) UnmarshalJSON(in []byte) (err os.Error) {
	var scopestr string
	err = json.Unmarshal(in, &scopestr)
	if err != nil {
		return err
	}
	switch scopestr {
	case "one":
		*js = SCOPE_ONEOF
	case "all":
		*js = SCOPE_ALLOF
	default:
		return InvalidValueError
	}
	return nil
}
