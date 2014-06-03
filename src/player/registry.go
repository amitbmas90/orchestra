// registry.go
//
// Job Registry.
//
// The Registry provides a 'threadsafe' interface to various global
// information stores.
//
// The registry dispatch thread is forbidden from performing any work
// that is likely to block.  Result channels must be buffered with
// enough space for the full set of results.

package main

const (
	requestAddTask			= iota
	requestGetTask

	requestQueueSize		= 10
)

type registryRequest struct {
	operation		int
	id			uint64
	task			*TaskRequest
	responseChannel		chan *registryResponse
}

type registryResponse struct {
	success			bool
	task			*TaskRequest
}

var chanRequest = make(chan *registryRequest, requestQueueSize)

// bake a minimal request structure together.
func newRequest(wants_response bool) (r *registryRequest) {
	r = new(registryRequest)
	if wants_response {
		r.responseChannel = make(chan *registryResponse, 1)
	}

	return r
}

// Add a Task to the registry.  Return true if successful, returns
// false if the task is lacking critical information (such as a Job Id)
// and can't be registered.
func TaskAdd(task *TaskRequest) bool {
	rr := newRequest(true)
	rr.operation = requestAddTask
	rr.task = task

	chanRequest <- rr
	resp := <-rr.responseChannel
	return resp.success
}

// Get a Task from the registry.  Returns the task if successful,
// returns nil if the task couldn't be found.
func TaskGet(id uint64) *TaskRequest {
	rr := newRequest(true)
	rr.operation = requestGetTask
	rr.id = id

	chanRequest <- rr
	resp := <-rr.responseChannel
	return resp.task
}

func manageRegistry() {
	taskRegister := make(map[uint64]*TaskRequest)

	for {
		req := <-chanRequest
		resp := new (registryResponse)
		switch (req.operation) {
		case requestAddTask:
			if nil != req.task {
				// and register the job
				taskRegister[req.task.Id] = req.task
				resp.success = true
			} else {
				resp.success = false
			}
		case requestGetTask:
			task, exists := taskRegister[req.id]
			resp.success = exists
			if exists {
				resp.task = task
			}
		}
		if req.responseChannel != nil {
			req.responseChannel <- resp
		}
	}
}

func init() {
	go manageRegistry()
}
