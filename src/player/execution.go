// execution.go

package main

import (
	"bufio"
	"io"
	"os"
	"strings"
	"syscall"
	o "orchestra"
)

func ExecuteTask(task *TaskRequest) <-chan *TaskResponse {
	complete  := make(chan *TaskResponse, 1)
	go doExecution(task, complete)

	return complete
}

func batchLogger(jobid uint64, errpipe *os.File) {
	defer errpipe.Close()

	r := bufio.NewReader(errpipe)
	for {
		lb, _, err := r.ReadLine()
		if err == io.EOF {
			return
		}
		if err != nil {
			o.Warn("executionLogger failed: %s", err)
			return
		}
		o.Info("JOB %d:STDERR:%s", jobid, string(lb))
	}
}

func peSetEnv(env []string, key string, value string) []string {
	mkey := key+"="
	found := false
	for i, v := range env {
		if strings.HasPrefix(v, mkey) {
			env[i] = key+"="+value
			found = true
			break
		}
	}
	if !found {
		env = append(env, key+"="+value)
	}
	return env
}

func doExecution(task *TaskRequest, completionChannel chan<- *TaskResponse) {
	// we must notify the parent when we exit.
	defer func(c chan<- *TaskResponse, task *TaskRequest) { c <- task.MyResponse }(completionChannel,task)

	// first of all, verify that the score exists at all.
	score, exists := Scores[task.Score]
	if !exists {
		o.Warn("job%d: Request for unknown score \"%s\"", task.Id, task.Score)
		task.MyResponse.State = RESP_FAILED_UNKNOWN_SCORE
		return
	}
	si := NewScoreInterface(task)
	if si == nil {
		o.Warn("job%d: Couldn't initialise Score Interface", task.Id)
		task.MyResponse.State = RESP_FAILED_HOST_ERROR
		return
	}
	if !si.Prepare() {
		o.Warn("job%d: Couldn't Prepare Score Interface", task.Id)
		task.MyResponse.State = RESP_FAILED_HOST_ERROR
		return
	}
	defer si.Cleanup()

	eenv := si.SetupProcess()
	task.MyResponse.State = RESP_RUNNING

	procenv := new(os.ProcAttr)
	// Build the default environment.
	procenv.Env = peSetEnv(procenv.Env, "PATH", "/usr/bin:/usr/sbin:/bin:/sbin")
	procenv.Env = peSetEnv(procenv.Env, "IFS", " \t\n")
	pwd, err := os.Getwd()
	if err != nil {
		task.MyResponse.State = RESP_FAILED_HOST_ERROR
		o.Warn("job%d: Couldn't resolve PWD: %s", task.Id, err)
		return
	}
	procenv.Env = peSetEnv(procenv.Env, "PWD", pwd)
	// copy in the environment overrides
	for k, v := range eenv.Environment {
		procenv.Env = peSetEnv(procenv.Env, k, v)
	}

	// attach FDs to procenv.
	procenv.Files = make([]*os.File, 3)

	// first off, attach /dev/null to stdin and stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR | os.O_APPEND, 0666)
	o.MightFail(err, "couldn't open DevNull")
	defer devNull.Close()
	for i := 0; i < 2; i++ {
		procenv.Files[i] = devNull
	}
	// attach STDERR to to our logger via pipe.
	lr, lw, err := os.Pipe()
	o.MightFail(err, "Couldn't create pipe")
	defer lw.Close()
	// lr will be closed by the logger.
	procenv.Files[2] = lw
	// check the environment's configuration and allow it to override stdin, stdout, and FDs 3+
	if nil != eenv.Files {
		for i := range eenv.Files {
			if i < 2 {
				procenv.Files[i] = eenv.Files[i]
			} else {
				procenv.Files = append(procenv.Files, eenv.Files[i])
			}
		}
	}
	var args []string = nil
	args = append(args, eenv.Arguments...)

	o.Info("job%d: Executing %s", task.Id, score.Executable)
	go batchLogger(task.Id, lr)
	proc, err := os.StartProcess(score.Executable, args, procenv)
	if err != nil {
		o.Warn("job%d: Failed to start processs", task.Id)
		task.MyResponse.State = RESP_FAILED_HOST_ERROR
		return
	}
	wm, err := proc.Wait()
	if err != nil {
		o.Warn("job%d: Error waiting for process", task.Id)
		task.MyResponse.State = RESP_FAILED_UNKNOWN
		// Worse of all, we don't even know if we succeeded.
		return
	}
	ws := wm.Sys()
	waitStatus := ws.(syscall.WaitStatus)

	if !(waitStatus.Signaled() || waitStatus.Exited()) {
		o.Assert("Non Terminal notification received when not expected.")
		return
	}
	if waitStatus.Signaled() {
		o.Warn("job%d: Process got signalled", task.Id)
		task.MyResponse.State = RESP_FAILED_UNKNOWN
		return
	}
	if waitStatus.Exited() {
		if 0 == waitStatus.ExitStatus() {
			o.Warn("job%d: Process exited OK", task.Id)
			task.MyResponse.State = RESP_FINISHED
		} else {
			o.Warn("job%d: Process exited with failure", task.Id)
			task.MyResponse.State = RESP_FAILED
		}
		return
	}
	o.Assert("Should never get here.")
}
