/* audience.go
 */

package main

import (
	"encoding/json"
	"io"
	"net"
	o "orchestra"
	"os"
	"path"
	"strings"
	"syscall"
)

type GenericJsonRequest struct {
	Op      *string           `json:"op"`
	Score   *string           `json:"score"`
	Players []string          `json:"players"`
	Scope   *JobScope         `json:"scope"`
	Params  map[string]string `json:"params"`
	Id      *uint64           `json:"id"`
}

type JsonPlayerStatus struct {
	Status   ResponseState     `json:"status"`
	Response map[string]string `json:"response"`
}

type JsonStatusResponse struct {
	Status  JobState                     `json:"status"`
	Players map[string]*JsonPlayerStatus `json:"players"`
}

func NewJsonStatusResponse() (jsr *JsonStatusResponse) {
	jsr = new(JsonStatusResponse)
	jsr.Players = make(map[string]*JsonPlayerStatus)

	return jsr
}

func NewJsonPlayerStatus() (jps *JsonPlayerStatus) {
	jps = new(JsonPlayerStatus)
	jps.Response = make(map[string]string)

	return jps
}

func handleAudienceRequest(c net.Conn) {
	defer c.Close()

	r, _ := c.(io.Reader)
	w, _ := c.(io.Writer)
	dec := json.NewDecoder(r)
	enc := json.NewEncoder(w)

	outobj := new(GenericJsonRequest)
	err := dec.Decode(outobj)
	if err != nil {
		o.Warn("Error decoding JSON request from audience: %s", err)
		return
	}

	if nil == outobj.Op {
		o.Warn("Malformed JSON request from audience: missing op")
		return
	}
	switch *(outobj.Op) {
	case "status":
		if nil == outobj.Id {
			o.Warn("Malformed status request from audience: missing job ID")
			return
		}
		job := JobGet(*outobj.Id)
		jresp := new([2]interface{})
		if nil != job {
			jresp[0] = "OK"
			iresp := NewJsonStatusResponse()
			iresp.Status = job.State
			resnames := JobGetResultNames(*outobj.Id)
			for i := range resnames {
				tr := JobGetResult(*outobj.Id, resnames[i])
				if nil != tr {
					presp := NewJsonPlayerStatus()
					presp.Status = tr.State
					for k, v := range tr.Response {
						presp.Response[k] = v
					}
					iresp.Players[resnames[i]] = presp
				}
			}
			jresp[1] = iresp
		} else {
			jresp[0] = "Error"
			jresp[1] = nil
		}
		enc.Encode(jresp)
	case "queue":
		if nil == outobj.Score {
			o.Warn("Malformed queue request from audience: missing score")
			sendQueueFailureResponse("Missing Score", enc)
			return
		}
		if nil == outobj.Scope {
			o.Warn("Malformed queue request from audience: missing scope")
			sendQueueFailureResponse("Missing Scope", enc)
			return
		}
		if nil == outobj.Players || len(outobj.Players) < 1 {
			o.Warn("Malformed queue request from audience: missing players")
			sendQueueFailureResponse("Missing Players", enc)
			return
		}
		for _, player := range outobj.Players {
			if !HostAuthorised(player) {
				o.Warn("Malformed queue request from audience: unknown player %s specified", player)
				sendQueueFailureResponse("Invalid Player", enc)
				return
			}
		}
		job := NewRequest()
		job.Score = *outobj.Score
		job.Scope = *outobj.Scope
		job.Players = outobj.Players
		job.Params = outobj.Params

		QueueJob(job)
		sendQueueSuccessResponse(job, enc)
	default:
		o.Warn("Unknown operation in request from audience: %s", *(outobj.Op))
		return
	}

	_ = enc
}

func sendQueueSuccessResponse(job *JobRequest, enc *json.Encoder) {
	resp := make([]interface{}, 2)
	resperr := new(string)
	*resperr = "OK"
	resp[0] = resperr

	// this probably looks odd, but all numbers cross through float64 when being json encoded.  d'oh!
	jobid := new(uint64)
	*jobid = uint64(job.Id)
	resp[1] = jobid

	err := enc.Encode(resp)
	if nil != err {
		o.Warn("Couldn't encode response to audience: %s", err)
	}
}

func sendQueueFailureResponse(reason string, enc *json.Encoder) {
	resp := make([]interface{}, 2)
	resperr := new(string)
	*resperr = "Error"
	resp[0] = resperr
	if reason != "" {
		resp[1] = &reason
	}
	err := enc.Encode(resp)
	if nil != err {
		o.Warn("Couldn't encode response to audience: %s", err)
	}
}

func AudienceListener(l net.Listener) {
	for {
		c, err := l.Accept()
		if err != nil {
			o.Warn("Accept() failed on Audience listener")
			break
		}
		go handleAudienceRequest(c)
	}
}

func UnixAudienceListener(sockaddr string) {
	fi, err := os.Stat(sockaddr)
	if err == nil {
		if (fi.Mode() & os.ModeSocket) != 0 {
			os.Remove(sockaddr)
		} else {
			o.Fail("%s exists and is not a socket", sockaddr)
		}
	}
	err = os.MkdirAll(path.Dir(sockaddr), 0755)
	o.MightFail(err, "Couldn't create socket directory")
	laddr, err := net.ResolveUnixAddr("unix", sockaddr)
	o.MightFail(err, "Couldn't resolve audience socket address")
	old_umask := syscall.Umask(0077)
	defer syscall.Umask(old_umask)
	l, err := net.ListenUnix("unix", laddr)
	o.MightFail(err, "Couldn't start audience unixsock listener")

	// make sure we clean up the unix socket when we die.
	defer l.Close()
	defer os.Remove(sockaddr)
	AudienceListener(l)
}

func StartAudienceSock() {
	audienceSockPath := strings.TrimSpace(GetStringOpt("audience socket path"))
	go UnixAudienceListener(audienceSockPath)
}
