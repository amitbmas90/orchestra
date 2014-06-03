// registry.go
//
// The Registry provides a 'threadsafe' interface to various global information stores.
//
// The registry dispatch thread is forbidden from performing any work that is likely to block.
// Result channels must be buffered with enough space for the full set of results.

package main

import (
	o "orchestra"
	"sort"
	"time"
	"container/list"
)

// Request Types
const (
	requestAddClient = iota
	requestGetClient
	requestDeleteClient
	requestSyncClients

	requestAddJob
	requestGetJob
	requestAddJobResult
	requestGetJobResult
	requestGetJobResultNames
	requestDisqualifyPlayer
	requestReviewJobStatus

	requestWriteJobUpdate
	requestWriteJobAll

	requestQueueSize		= 10

	jobLingerTime			= int64(30e9)
)

type registryRequest struct {
	operation		int
	id			uint64
	hostname		string
	hostlist		[]string
	job			*JobRequest
	tresp			*TaskResponse
	responseChannel		chan *registryResponse
}

type registryResponse struct {
	success			bool
	info			*ClientInfo
	tresp			*TaskResponse
	names			[]string
	jobs			[]*JobRequest
}

var (
	chanRegistryRequest	= make(chan *registryRequest, requestQueueSize)
	clientList		= make(map[string]*ClientInfo)
	jobRegister		= make(map[uint64]*JobRequest)
	expiryChan		<-chan int64
	expiryJobid		uint64
	expiryList		*list.List

	expiryLoopFudge		int64 = 10e6; /* 10 ms should be enough fudgefactor */
)

func init() {
	expiryList = list.New()
}

func regInternalAdd(hostname string) {
	o.Warn("Registry: New Host \"%s\"", hostname)
	clientList[hostname] = NewClientInfo()
	// do this initialisation here since it'll help unmask sequencing errors
	clientList[hostname].pendingTasks = make(map[uint64]*TaskRequest)
	clientList[hostname].Player = hostname
}

func regInternalDel(hostname string) {
	o.Warn("Registry: Deleting Host \"%s\"", hostname)
	/* remove it from the registry */
	clientList[hostname] = nil, false
}

func regInternalExpireJob(jobid uint64) {
	job, exists := jobRegister[jobid]
	if exists {
		if job.State.Finished() {
			jobRegister[jobid] = nil, false
		} else {
			o.Assert("Tried to expire incomplete job.")
		}
	}
}

func regInternalFindNextExpiry() {
	if expiryChan != nil {
		o.Assert("Attempted to Find Next Expiry avenue with expiry timer active.")
	}
	// if there's nothing to expire, do nothing.
	if expiryList.Len() == 0 {
		return
	}

	for expiryChan == nil && expiryList.Len() > 0 {
		jobif := expiryList.Remove(expiryList.Front())
		req, ok := jobif.(*JobRequest)
		if !ok {
			o.Assert("item in expiryList not a *JobRequest")
		}
		if (time.Nanoseconds() + expiryLoopFudge) > req.expirytime {
			regInternalExpireJob(req.Id)
		} else {
			expiryChan = time.After(req.expirytime - time.Nanoseconds())
			expiryJobid = req.Id
		}
	}
}

func regInternalMarkJobForExpiry(job *JobRequest) {
	job.expirytime = time.Nanoseconds() + jobLingerTime
	expiryList.PushBack(job)
	// if there is no job pending expiry, feed it into the delay loop
	if expiryChan == nil {
		regInternalFindNextExpiry()
	}
}

var registryHandlers = map[int] func(*registryRequest, *registryResponse) {
	requestAddClient:		regintAddClient,
	requestGetClient:		regintGetClient,
	requestDeleteClient:		regintDeleteClient,
	requestSyncClients:		regintSyncClients,
	requestAddJob:			regintAddJob,
	requestGetJob:			regintGetJob,
	requestAddJobResult:		regintAddJobResult,
	requestGetJobResult:		regintGetJobResult,
	requestGetJobResultNames:	regintGetJobResultNames,
	requestDisqualifyPlayer:	regintDisqualifyPlayer,
	requestReviewJobStatus:		regintReviewJobStatus,
	requestWriteJobUpdate:		regintWriteJobUpdate,
	requestWriteJobAll:		regintWriteJobAll,
}

func manageRegistry() {
	for {
		select {
		case req := <-chanRegistryRequest:
			resp := new(registryResponse)
			// by default, we failed.
			resp.success = false
			// find the operation
			handler, exists := registryHandlers[req.operation]
			if exists {
				handler(req, resp)
			}
			if req.responseChannel != nil {
				req.responseChannel <- resp
			}
		case <-expiryChan:
			o.Debug("job%d: Expiring Job Record", expiryJobid)
			regInternalExpireJob(expiryJobid)
			expiryChan = nil
			regInternalFindNextExpiry()
		}
	}
}

func StartRegistry() {
	go manageRegistry()
}

func newRequest(wants_response bool) (req *registryRequest) {
	req = new(registryRequest)
	if wants_response {
		req.responseChannel = make(chan *registryResponse, 1)
	}

	return req
}

func ClientAdd(hostname string) (success bool) {
	r := newRequest(true)
	r.operation = requestAddClient
	r.hostname = hostname
	chanRegistryRequest <- r
	resp := <-r.responseChannel

	return resp.success
}

func regintAddClient(req *registryRequest, resp *registryResponse) {
	_, exists := clientList[req.hostname]
	if exists {
		resp.success = false
	} else {
		regInternalAdd(req.hostname)
		resp.success = true
	}
}

func ClientDelete(hostname string) (success bool) {
	r := newRequest(true)
	r.operation = requestDeleteClient
	r.hostname = hostname
	chanRegistryRequest <- r
	resp := <-r.responseChannel

	return resp.success
}

func regintDeleteClient(req *registryRequest, resp *registryResponse) {
	_, exists := clientList[req.hostname]
	if exists {
		resp.success = true
		regInternalDel(req.hostname)
	} else {
		resp.success = false
	}
}

func ClientGet(hostname string) (info *ClientInfo) {
	r := newRequest(true)
	r.operation = requestGetClient
	r.hostname = hostname
	chanRegistryRequest <- r
	resp := <-r.responseChannel
	if resp.success {
		return resp.info
	}
	return nil
}

func regintGetClient(req *registryRequest, resp *registryResponse) {
	clinfo, exists := clientList[req.hostname]
	if exists {
		resp.success = true
		resp.info = clinfo
	} else {
		resp.success = false
	}
}

func ClientUpdateKnown(hostnames []string) {
	/* this is an asynchronous, we feed it into the registry 
	* and it'll look after itself.
	*/
	r := newRequest(false)
	r.operation = requestSyncClients
	r.hostlist = hostnames
	chanRegistryRequest <- r
}

func regintSyncClients(req *registryRequest, resp *registryResponse) {
	// we need to make sure the registered clients matches the
	// hostlist we're given.
	//
	// First, we transform the array into a map
	newhosts := make(map[string]bool)
	for k,_ := range req.hostlist {
		newhosts[req.hostlist[k]] = true
	}
	// now, scan the current list, checking to see if they exist.
	// Remove them from the newhosts map if they do exist.
	for k,_ := range clientList {
		_, exists := newhosts[k]
		if exists {
			// remove it from the newhosts map
			newhosts[k] = false, false
		} else {
			regInternalDel(k)
		}
	}
	// now that we're finished, we should only have new clients in
	// the newhosts list left.
	for k,_ := range newhosts {
		regInternalAdd(k)
	}
	// and we're done.
}

// Add a Job to the registry.  Return true if successful, returns
// false if the job is lacking critical information (such as a JobId)
// and can't be registered.
func JobAdd(job *JobRequest) bool {
	rr := newRequest(true)
	rr.operation = requestAddJob
	rr.job = job

	chanRegistryRequest <- rr
	resp := <-rr.responseChannel
	return resp.success
}

func regintAddJob(req *registryRequest, resp *registryResponse) {
	if nil == req.job {
		return
	}
	// ensure that the players are sorted!
	sort.Strings(req.job.Players)
	// update the state
	req.job.updateState()
	// and register the job
	_, overwrite := jobRegister[req.job.Id]
	if !overwrite {
		jobRegister[req.job.Id] = req.job
		// force a queue update.
		req.job.UpdateInSpool()
		if req.job.State.Finished() {
			regInternalMarkJobForExpiry(req.job)
		}
		resp.success = true
	}
}

// Get a Job from the registry.  Returns the job if successful,
// returns nil if the job couldn't be found.
func JobGet(id uint64) *JobRequest {
	rr := newRequest(true)
	rr.operation = requestGetJob
	rr.id = id

	chanRegistryRequest <- rr
	resp := <-rr.responseChannel
	if resp.jobs == nil {
		return nil
	}
	return resp.jobs[0]
}

func regintGetJob(req *registryRequest, resp *registryResponse) {
	job, exists := jobRegister[req.id]
	resp.success = exists
	if exists {
		resp.jobs = make([]*JobRequest, 1)
		resp.jobs[0] = job
	} else {
		o.Warn("Received Request for job%d which is not in memory", req.id)
		go regintGetJobDeferred(req.id, req.responseChannel)
		// mask out the responseChannel so the deferred handler can use it.
		req.responseChannel = nil
	}
}

func regintGetJobDeferred(jobid uint64, responseChannel chan<- *registryResponse) {
	resp := new(registryResponse)
	resp.success = false
	defer func (resp *registryResponse, rChan chan<- *registryResponse) {
		rChan <- resp;
	}(resp, responseChannel)

	req, err := LoadFromFinished(jobid)
	if err != nil {
		o.Warn("Couldn't load job%d from disk.  Doesn't exist?", jobid)
		return
	}
	// fix up the state, and stuff it back into the system
	RestoreJobState(req)
	resp.jobs = make([]*JobRequest, 1)
	resp.jobs[0] = req
	resp.success = true
}

// Attach a result to a Job in the Registry
//
// This exists in order to prevent nasty concurrency problems
// when trying to put results back onto the job.  Reading a job is far
// less of a problem than writing to it.
func JobAddResult(playername string, task *TaskResponse) bool {
	rr := newRequest(true)
	rr.operation = requestAddJobResult
	rr.tresp = task
	rr.hostname = playername
	chanRegistryRequest <- rr
	resp := <-rr.responseChannel
	return resp.success
}

func regintAddJobResult(req *registryRequest, resp *registryResponse) {
	job, exists := jobRegister[req.tresp.id]
	resp.success = exists
	if exists {
		job.Results[req.hostname] = req.tresp
		// force a queue update.
		job.UpdateInSpool()
	}
}

// Get a result from the registry
func JobGetResult(id uint64, playername string) (tresp *TaskResponse) {
	rr := newRequest(true)
	rr.operation = requestGetJobResult
	rr.id = id
	rr.hostname = playername
	chanRegistryRequest <- rr
	resp := <-rr.responseChannel
	return resp.tresp
}

func regintGetJobResult(req *registryRequest, resp *registryResponse) {
	job, exists := jobRegister[req.id]
	if exists {
		result, exists := job.Results[req.hostname]
		resp.success = exists
		if exists {
			resp.tresp = result
		}
	} else {
		resp.success = false
	}
}

// Get a list of names we have results for against a given job.
func JobGetResultNames(id uint64) (names []string) {
	rr := newRequest(true)
	rr.operation = requestGetJobResultNames
	rr.id = id

	chanRegistryRequest <- rr
	resp := <-rr.responseChannel
	return resp.names
}

func regintGetJobResultNames(req *registryRequest, resp *registryResponse) {
	job, exists := jobRegister[req.id]
	resp.success = exists
	if exists {
		resp.names = make([]string, len(job.Results))
		idx := 0
		for k, _ := range job.Results {
			resp.names[idx] = k
			idx++
		}
	}
}

//  Disqualify a player from servicing a job
func JobDisqualifyPlayer(id uint64, playername string) bool {
	rr := newRequest(true)
	rr.operation = requestDisqualifyPlayer
	rr.id = id
	rr.hostname = playername

	chanRegistryRequest <- rr
	resp := <-rr.responseChannel

	return resp.success
}

func regintDisqualifyPlayer(req *registryRequest, resp *registryResponse) {
	job, exists := jobRegister[req.id]
	if exists {
		idx := sort.Search(len(job.Players), func(idx int) bool { return job.Players[idx] >= req.hostname })
		if (job.Players[idx] == req.hostname) {
			resp.success = true
			newplayers := make([]string, len(job.Players)-1)
			copy(newplayers[0:idx], job.Players[0:idx])
			copy(newplayers[idx:len(job.Players)-1], job.Players[idx+1:len(job.Players)])
			job.Players = newplayers
			job.updateState()
			// force a queue update.
			job.UpdateInSpool()
		} else {
			resp.success = false
		}
	} else {
		resp.success = false
	}
}

func JobReviewState(id uint64) bool {
	rr := newRequest(true)
	rr.operation = requestReviewJobStatus
	rr.id = id

	chanRegistryRequest <- rr
	resp := <-rr.responseChannel

	return resp.success
}

func regintReviewJobStatus(req *registryRequest, resp *registryResponse) {
	job, exists := jobRegister[req.id]
	resp.success = exists
	if exists {
		job.updateState()
		// force a queue update.
		job.UpdateInSpool()
	}
}

func JobWriteUpdate(id uint64) {
	rr := newRequest(false)
	rr.operation = requestWriteJobUpdate
	rr.id = id
	chanRegistryRequest <- rr
}

func regintWriteJobUpdate(req *registryRequest, resp *registryResponse) {
	job, exists := jobRegister[req.id]
	resp.success = exists
	if exists {
		job.UpdateInSpool()
	}
}

func JobWriteAll() bool {
	rr := newRequest(true)
	rr.operation = requestWriteJobAll

	chanRegistryRequest <- rr
	resp := <-rr.responseChannel

	return resp.success
}

func regintWriteJobAll(req *registryRequest, resp *registryResponse) {
	for _, job := range jobRegister {
		job.UpdateInSpool()
	}
	resp.success = true
}

// Ugh.
func (job *JobRequest) updateState() {
	if job.Results == nil {
		o.Assert("job.Results nil for jobid %d", job.Id)
		return
	}
	was_finished := job.State.Finished()
	switch job.Scope {
	case SCOPE_ONEOF:
		// look for a success (any success) in the responses
		var success bool = false
		for host, res := range job.Results {
			if res == nil {
				o.Debug("nil result for %s?", host)
				continue
			}
			if res.State == RESP_FINISHED {
				success = true
				break
			}
		}
		// update the job state based upon these findings
		if success {
			job.State = JOB_SUCCESSFUL
		} else {
			if len(job.Players) < 1 {
				job.State = JOB_FAILED
			} else {
				job.State = JOB_PENDING
			}
		}
	case SCOPE_ALLOF:
		var success int = 0
		var failed  int = 0

		for pidx := range job.Players {
			p := job.Players[pidx]
			resp, exists := job.Results[p]
			if exists {
				if resp.DidFail() {
					failed++
				} else if resp.State == RESP_FINISHED {
					success++
				}
			}
		}
		if (success + failed) < len(job.Players) {
			job.State = JOB_PENDING
		} else if success == len(job.Players) {
			job.State = JOB_SUCCESSFUL
		} else if failed == len(job.Players) {
			job.State = JOB_FAILED
		} else {
			job.State = JOB_FAILED_PARTIAL
		}
	}
	if !was_finished && job.State.Finished() {
		o.Debug("job%d: Finished - Setting Expiry Time", job.Id)
		regInternalMarkJobForExpiry(job)
	}
}
