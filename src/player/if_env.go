// if_env
//
// 'env' score interface

package main

const (
	envEnvironmentPrefix = "ORC_"
)

func init() {
	RegisterInterface("env", newEnvInterface)
}

type EnvInterface struct {
	task *TaskRequest
}

func newEnvInterface(task *TaskRequest) (iface ScoreInterface) {
	ei := new(EnvInterface)
	ei.task = task

	return ei
}

func (ei *EnvInterface) Prepare() bool {
	// does nothing!
	return true
}

func (ei *EnvInterface) SetupProcess() (ee *ExecutionEnvironment) {
	ee = NewExecutionEnvironment()
	for k, v := range ei.task.Params {
		ee.Environment[envEnvironmentPrefix+k] = v
	}

	return ee
}

func (ei *EnvInterface) Cleanup() {
	// does nothing!
}
