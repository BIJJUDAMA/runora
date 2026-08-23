package runner

// OnnxRuntime coordinates ONNX model execution through the ProcessSupervisor and OnnxDriver.
type OnnxRuntime struct {
	supervisor *ProcessSupervisor
	driver     *OnnxDriver
}

func NewOnnxRuntime(logDir string) *OnnxRuntime {
	return &OnnxRuntime{
		supervisor: NewProcessSupervisor(logDir),
		driver:     NewOnnxDriver(),
	}
}

func NewOnnxRuntimeWithSupervisor(supervisor *ProcessSupervisor) *OnnxRuntime {
	return &OnnxRuntime{
		supervisor: supervisor,
		driver:     NewOnnxDriver(),
	}
}

func (or *OnnxRuntime) Start(modelPath string, opts StartOptions) error {
	_, err := or.supervisor.StartInstance(or.driver, modelPath, opts)
	return err
}

func (or *OnnxRuntime) Stop() error {
	return or.supervisor.Stop()
}

func (or *OnnxRuntime) StopInstance(port int) error {
	return or.supervisor.StopInstance(port)
}

func (or *OnnxRuntime) GetStatus() (ServerStatus, string, int) {
	return or.supervisor.GetStatus()
}

func (or *OnnxRuntime) GetAllInstances() []InstanceInfo {
	return or.supervisor.GetAllInstances()
}

func (or *OnnxRuntime) Capabilities() []TaskType {
	return or.driver.Capabilities()
}
