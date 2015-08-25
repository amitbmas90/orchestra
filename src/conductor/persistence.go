// persistence.go
//
package main

import (
	"fmt"
	"path"
	"os"
	"unicode"
	"strconv"
	"sync/atomic"
	"bufio"
	"strings"
	o "orchestra"
)

// changing this will result in fire.  you have been warned.
const bucketDepth = 2

var spoolDirectory = ""

func SetSpoolDirectory(spooldir string) {
	if spoolDirectory == "" {
		spoolDirectory = spooldir
	} else {
		if spooldir != spoolDirectory {
			o.Warn("Spool Directory Not Changed.")
		}
	}
}

func GetSpoolDirectory() string {
	if spoolDirectory == "" {
		o.Assert("GetSpoolDirectory() called before set")
	}
	return spoolDirectory
}


const (
	IdCheckpointSafetySkip = 10e4  // Skip 10e4 entries if orchestra didn't shutdown cleanly for safety.
)

var lastId uint64 = 0

func checkpointPath() string {
	return path.Join(spoolDirectory, "last_id.checkpoint")
}

func savePath() string {
	return path.Join(spoolDirectory, "last_id")
}

func loadLastId() {
	fh, err := os.Open(checkpointPath())
	if err == nil {
		defer fh.Close()

		// we have a checkpoint file.  blah.
		cbio := bufio.NewReader(fh)
		l, err := cbio.ReadString('\n')
		lastId, err = strconv.Atoui64(strings.TrimSpace(l))
		if err != nil {
			o.Fail("Couldn't read Last ID from checkpoint file.  Aborting for safety.")
		}
		lastId += IdCheckpointSafetySkip
	} else {
		if !os.IsNotExist(err) {
			o.Fail("Found checkpoint file, but couldn't open it: %s", err)
		}
		fh,err := os.Open(savePath())
		if err != nil {
			if os.IsNotExist(err) {
				lastId = 0;
				return;
			}
			o.MightFail(err, "Couldn't open last_id file")
		}
		defer fh.Close()
		cbio := bufio.NewReader(fh)
		l, err := cbio.ReadString('\n')
		lastId, err = strconv.Atoui64(strings.TrimSpace(l))
		if err != nil {
			o.Fail("Couldn't read Last ID from last_id.  Aborting for safety.")
		}
	}
	writeIdCheckpoint()
}

func writeIdCheckpoint() {
	fh, err := os.OpenFile(checkpointPath(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		o.Warn("Failed to create checkpoint file: %s", err)
		return
	}
	defer fh.Close()
	fmt.Fprintf(fh, "%d\n", lastId)
}

func saveLastId() {
	fh, err := os.OpenFile(savePath(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		o.Warn("Failed to create last ID save file: %s", err)
		return
	}
	defer fh.Close()
	fmt.Fprintf(fh, "%d\n", lastId)
	os.Remove(checkpointPath())
}

func NextRequestId() uint64 {
	//FIXME: we should do this periodically, not on every new job.
	defer writeIdCheckpoint()
	return atomic.AddUint64(&lastId, 1)
}

func FilenameForJobId(jobid uint64) (fullpath string) {
	fnbits := make([]string, bucketDepth+1)
	for i := 0; i < bucketDepth; i++ {
		fnbits[i] = fmt.Sprintf("%01X", (jobid >> uint(i*4)) & 0xF)
	}
	fnbits[bucketDepth] = fmt.Sprintf("%016X", jobid)

	return path.Join(fnbits...)
}

func makeSpoolDirInner(prefix string, depth int) {
	for i := 0; i < 16; i++ {
		dirname := path.Join(prefix, fmt.Sprintf("%01X", i))
		if (depth == 1) {
			err := os.MkdirAll(dirname, 0700)
			o.MightFail(err, "Couldn't make directory building spool tree")
		} else {
			makeSpoolDirInner(dirname, depth-1)
		}
	}
}

func MakeSpoolDir() {
	makeSpoolDirInner(path.Join(spoolDirectory, "active"), bucketDepth)
	makeSpoolDirInner(path.Join(spoolDirectory, "finished"), bucketDepth)
	os.MkdirAll(path.Join(spoolDirectory, "corrupt"), 0700)
}

func UnlinkNodesForJobId(jobid uint64) {
	suffix := FilenameForJobId(jobid)

	os.Remove(path.Join(spoolDirectory, "active", suffix))
	os.Remove(path.Join(spoolDirectory, "finished", suffix))
}

func shuffleToCorrupted(abspath, reason string) {
	basename := path.Base(abspath)
	targetname := path.Join(spoolDirectory, "corrupt", basename)
	// make sure there's nothing in the target name.
	os.Remove(targetname)
	err := os.Rename(abspath, targetname)
	o.MightFail(err, "Couldn't bin corrupt spoolfile %s", abspath)
	o.Warn("Moved \"%s\" to corrupted spool: %s", abspath, reason)
}

func loadSpoolFiles(dirname string, depth int) {
	dh, err := os.Open(dirname)
	o.MightFail(err, "Couldn't open %s", dirname)
	nodes, err := dh.Readdir(-1)
	o.MightFail(err, "Couldn't readdir on %s", dirname)
	if depth > 0 {
		for _, n := range nodes {
			abspath := path.Join(dirname, n.Name())
			if n.Mode().IsDir() {
				// if not a single character, it's not a spool node.
				if len(n.Name()) != 1 {
					continue;
				}
				if n.Name() == "." {
					// we're not interested in .
					continue;
				}
				nrunes := []rune(n.Name())
				if unicode.Is(unicode.ASCII_Hex_Digit, nrunes[0]) {
					loadSpoolFiles(abspath, depth-1)
				} else {
					o.Warn("Foreign dirent %s found in spool tree", abspath)
				}
			}
		}
	} else {
		// depth == 0 - only interested in files.
		for _, n := range nodes {
			abspath := path.Join(dirname, n.Name())
			if n.Mode().IsRegular() {
				if len(n.Name()) != 16 {
					shuffleToCorrupted(abspath, "Filename incorrect length")
					continue
				}
				id, err := strconv.Btoui64(n.Name(), 16)
				if err != nil {
					shuffleToCorrupted(abspath, "Invalid Filename")
					continue
				}
				fh, err := os.Open(abspath)
				if err != nil {
					shuffleToCorrupted(abspath, "Couldn't open")
					continue
				}
				defer fh.Close()
				jr, err := JobRequestFromReader(fh)
				if err != nil || jr.Id != id {
					o.Warn("Couldn't parse?! %s", err)
					shuffleToCorrupted(abspath, "Parse Failure")
					continue
				}
				// Add the request to the registry directly.
				if !RestoreJobState(jr) {
					shuffleToCorrupted(abspath, "Job State Invalid")
				}
			}
		}
	}
}

// This takes an unmarshall'd job and stuffs it back into the job state.
func RestoreJobState(job *JobRequest) bool {
	// check the valid players list.
	var playersout []string = nil
	resultsout := make(map[string]*TaskResponse)
	for _, p := range job.Players {
		if HostAuthorised(p) {
			playersout = append(playersout, p)
			// fix the result too.
			resout, exists := job.Results[p]
			if exists && resout != nil {
				resout.id = job.Id
				resultsout[p] = resout
			}
			// remove it so we can sweep it in pass2 for
			// results from old hosts that matter.
			delete(job.Results, p)
		}
	}
	job.Players = playersout
	if len(job.Players) == 0 {
		// If there are no players left at this point, discard
		// the job as corrupt.
		return false
	}
	// now, do pass 2 over the remaining results.
	for k, v := range job.Results {
		if v != nil {
			// if the results indicate completion, we
			// always retain them.
			if v.State.Finished() {
				resultsout[k] = v
				resultsout[k].id = job.Id
			}
		}
	}
	job.Results = resultsout

	// now, check the task data.  ONEOF jobs are allowed to
	// reset tasks that have never been sent.
	var tasksout []*TaskRequest = nil
	for _, t := range job.Tasks {
		// rebuild the return link
		t.job = job
		// finished tasks we don't care about.
		if t.State.Finished() {
			tasksout = append(tasksout, t)
			continue
		}
		if job.Scope == SCOPE_ONEOF {
			if t.Player != "" && (t.State == TASK_QUEUED || !HostAuthorised(t.Player)) {
				t.State = TASK_QUEUED
				t.Player = ""
			}
			tasksout = append(tasksout, t)
			continue
		} else {
			if HostAuthorised(t.Player) {
				tasksout = append(tasksout, t)
			}
		}
	}
	job.Tasks = tasksout
	if len(job.Tasks) == 0 {
		o.Debug("Empty tasks in deserialised job")
		// Tasks should never be empty.
		return false
	}
	// put the job back into the system.
	JobAdd(job)
	JobReviewState(job.Id)
	if (!job.State.Finished()) {
		// now, redispatch anything that's not actually finished.
		for _, t := range job.Tasks {
			if !t.State.Finished() {
				DispatchTask(t)
			}
		}
	}
	return true
}

func LoadState() {
	loadLastId()
	dirname := path.Join(spoolDirectory, "active")
	loadSpoolFiles(dirname, bucketDepth)
}

func SaveState() {
	JobWriteAll()
	saveLastId()
}
