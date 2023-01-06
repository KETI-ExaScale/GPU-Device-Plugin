package nvidia

import (
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/NVIDIA/go-gpuallocator/gpuallocator"
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceName           = "keti.com/mpsgpu" //여기서 지정
	serverSock             = pluginapi.DevicePluginPath + "nvidia-keti-mpsgpu.sock"
	envDisableHealthChecks = "DP_DISABLE_HEALTHCHECKS"
	allHealthChecks        = "xids"
)

/*func getFrame(skipFrames int) runtime.Frame {
	// We need the frame at index skipFrames+2, since we never want runtime.Callers and getFrame
	targetFrameIndex := skipFrames + 2

	// Set size to targetFrameIndex+2 to ensure we have room for one more caller than we need
	programCounters := make([]uintptr, targetFrameIndex+2)
	n := runtime.Callers(0, programCounters)

	frame := runtime.Frame{Function: "unknown"}
	if n > 0 {
		frames := runtime.CallersFrames(programCounters[:n])
		for more, frameIndex := true, 0; more && frameIndex <= targetFrameIndex; frameIndex++ {
			var frameCandidate runtime.Frame
			frameCandidate, more = frames.Next()
			if frameIndex == targetFrameIndex {
				frame = frameCandidate
			}
		}
	}

	return frame
}

// MyCaller returns the caller of the function that called it :)
func MyCaller() string {
	// Skip GetCallerFunctionName and the function to get the caller of
	return getFrame(2).Function
}*/

// NvidiaDevicePlugin implements the Kubernetes device plugin API
type NvidiaDevicePlugin struct {
	devs         []*pluginapi.Device
	physicalDevs []string

	socket         string
	allocatePolicy gpuallocator.Policy
	stop           chan interface{}
	health         chan *pluginapi.Device

	server *grpc.Server
}

// NewNvidiaDevicePlugin returns an initialized NvidiaDevicePlugin
func NewNvidiaDevicePlugin(vGPUCount int) *NvidiaDevicePlugin {
	physicalDevs := getPhysicalGPUDevices()
	vGPUDevs := getVGPUDevices(vGPUCount)

	return &NvidiaDevicePlugin{
		devs:         vGPUDevs,
		physicalDevs: physicalDevs,
		socket:       serverSock,

		stop:   make(chan interface{}),
		health: make(chan *pluginapi.Device),
	}
}

func (m *NvidiaDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

// dial establishes the gRPC communication with the registered device plugin.
func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

// Start starts the gRPC server of the device plugin
func (m *NvidiaDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go func() {
		lastCrashTime := time.Now()
		restartCount := 0
		for {
			log.Println("Starting GRPC server")
			err := m.server.Serve(sock)
			if err != nil {
				log.Printf("GRPC server crashed with error: %v", err)
			}
			// restart if it has not been too often
			// i.e. if server has crashed more than 5 times and it didn't last more than one hour each time
			if restartCount > 5 {
				// quit
				log.Fatal("GRPC server has repeatedly crashed recently. Quitting")
			}
			timeSinceLastCrash := time.Since(lastCrashTime).Seconds()
			lastCrashTime = time.Now()
			if timeSinceLastCrash > 3600 {
				// it has been one hour since the last crash.. reset the count
				// to reflect on the frequency
				restartCount = 1
			} else {
				restartCount += 1
			}
		}
	}()

	// Wait for server to start by launching a blocking connexion
	conn, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	// go m.healthcheck()

	return nil
}

// Stop stops the gRPC server
func (m *NvidiaDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)

	return m.cleanup()
}

// Register registers the device plugin for the given resourceName with Kubelet.
func (m *NvidiaDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
	fmt.Println("registered resourceName :", resourceName)
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		log.Printf("endpoint %s, Dial conn error: %s", kubeletEndpoint, err)
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		log.Printf("client register: %s", err)
		return err
	}
	return nil
}

// ListAndWatch lists devices and update that list according to the health status
func (m *NvidiaDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})

	for {
		//fmt.Println("listandwatch")
		select {
		case <-m.stop:
			return nil
		case d := <-m.health:
			// FIXME: there is no way to recover from the Unhealthy state.
			d.Health = pluginapi.Unhealthy
			log.Printf("device marked unhealthy: %s", d.ID)
			//fmt.Println("listandwatch1")
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
		}
	}
}

func (m *NvidiaDevicePlugin) unhealthy(dev *pluginapi.Device) {
	m.health <- dev
}

func getpendingpodslist() (*v1.PodList, error) {
	host_config, _ := rest.InClusterConfig()
	host_kubeClient := kubernetes.NewForConfigOrDie(host_config)
	MY_NODENAME := os.Getenv("MY_NODE_NAME")
	selector := fields.SelectorFromSet(fields.Set{"spec.nodeName": MY_NODENAME, "status.phase": "Pending"})
	podlist, err := host_kubeClient.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{
		FieldSelector: selector.String(),
		LabelSelector: labels.Everything().String(),
	})
	for i := 0; i < 3 && err != nil; i++ {
		podlist, err = host_kubeClient.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{
			FieldSelector: selector.String(),
			LabelSelector: labels.Everything().String(),
		})
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Pods assigned to node %v", MY_NODENAME)
	}
	return podlist, nil
}

// var onepodnum = 0
var podname = ""

// Allocate which return list of devices.
func (m *NvidiaDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	//여기가 메인임
	//fmt.Println("allocate")
	//fmt.Println(MyCaller())
	//fmt.Println(reqs.ContainerRequests)
	devs := m.devs
	responses := pluginapi.AllocateResponse{}
	allocatenum := 0
	var UUIDs string
	var UUID []string
	podlist, err := getpendingpodslist()
	pods := []v1.Pod{}
	MY_NODENAME := os.Getenv("MY_NODE_NAME")
	for _, pod := range podlist.Items {
		if pod.Spec.NodeName != MY_NODENAME {
			fmt.Println("Node Name ERROR")
		} else {
			pods = append(pods, pod)
		}
	}
	var assumePod v1.Pod
	var limits string
	for i := 0; i < 10; i++ {
		for _, pod := range pods {
			assumePod = pod
			UUIDIndex := "UUID"
			if len(assumePod.ObjectMeta.Annotations) > 0 {
				UUIDs = assumePod.ObjectMeta.Annotations[UUIDIndex]
				limits = assumePod.ObjectMeta.Annotations["GPUlimits"]
			}
		}
		if podname != assumePod.ObjectMeta.Name {
			podname = assumePod.ObjectMeta.Name
			break
		} else {
			podlist, _ := getpendingpodslist()
			for _, pod := range podlist.Items {
				if pod.Spec.NodeName != MY_NODENAME {
					fmt.Println("Node Name ERROR")
				} else {
					pods = append(pods, pod)
				}
			}
		}
		time.Sleep(100000000)
	}

	if assumePod.Namespace != "gpu" { //User Pod UUID에 할당
		if err != nil {
			fmt.Printf("pendingpod error\n")
		}
		UUID = strings.Split(UUIDs, ",")
		fmt.Println("----------------GPU Scheduler 선정 GPU----------------")
		fmt.Printf("pod name : %v\n", assumePod.Name)
		fmt.Printf("Pod Annotation UUID : %v\n", UUID)
		physicalDevsMap := make(map[string]bool)
		for _, req := range reqs.ContainerRequests {
			for _, id := range req.DevicesIDs {
				if !deviceExists(devs, id) {
					return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
				}

				// Convert virtual GPUDeviceId to physical GPUDeviceID
				physicalDevId := getPhysicalDeviceID(id)
				//physicalDevId = "GPU-f6db4146-092d-146f-0814-8ff90b04f3d2-1 ~ -10"
				if !physicalDevsMap[physicalDevId] {
					physicalDevsMap[physicalDevId] = true
				}

				dev := getDeviceById(devs, id)
				if dev == nil {
					return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
				}

				if dev.Health != pluginapi.Healthy {
					return nil, fmt.Errorf("invalid allocation request with unhealthy device %s", id)
				}
			}
			// if len(req.DevicesIDs) > 0 {
			// 	UUID = append(UUID, "GPU-a06cd524-72c4-d6f0-4eda-d64af512dd8b")
			// 	// 	UUID = append(UUID, "GPU-f6db4146-092d-146f-0814-8ff90b04f3d2")
			// 	// 	UUID[0] = "GPU-a06cd524-72c4-d6f0-4eda-d64af512dd8b"
			// 	// 	UUID[1] = "GPU-f6db4146-092d-146f-0814-8ff90b04f3d2"
			// }
			visibleDevs := make([]string, 0, len(physicalDevsMap))
			visibleDev := ""
			limitDevs := make([]string, 0, len(physicalDevsMap))
			limitDev := ""
			for i := 0; i < len(req.DevicesIDs); i++ {
				visibleDev = UUID[i]
				if limits != "" {
					limitDev = UUID[i] + "=" + limits
					limitDevs = append(limitDevs, limitDev)
				}
				// if len(req.DevicesIDs) > 1 {
				// 	if i == 0 {
				// 		visibleDev = "GPU-a06cd524-72c4-d6f0-4eda-d64af512dd8b"
				// 	} else {
				// 		visibleDev = "GPU-f6db4146-092d-146f-0814-8ff90b04f3d2"
				// 	}
				// } else {
				// 	if onepodnum%2 == 0 {
				// 		visibleDev = "GPU-a06cd524-72c4-d6f0-4eda-d64af512dd8b"
				// 	} else {
				// 		visibleDev = "GPU-f6db4146-092d-146f-0814-8ff90b04f3d2"
				// 	}
				// 	onepodnum++
				// }
				visibleDevs = append(visibleDevs, visibleDev)
			}
			fmt.Println("----------------GPU Device Plugin 할당 GPU----------------")
			fmt.Printf("User Pod using GPU UUID : %v\n", visibleDevs)
			if limits != "" {
				response := pluginapi.ContainerAllocateResponse{
					Envs: map[string]string{
						"NVIDIA_VISIBLE_DEVICES":           strings.Join(visibleDevs, ","),
						"CUDA_MPS_PINNED_DEVICE_MEM_LIMIT": strings.Join(limitDevs, ","),
					},
				}

				responses.ContainerResponses = append(responses.ContainerResponses, &response)
			} else {
				response := pluginapi.ContainerAllocateResponse{
					Envs: map[string]string{
						"NVIDIA_VISIBLE_DEVICES": strings.Join(visibleDevs, ","),
					},
				}

				responses.ContainerResponses = append(responses.ContainerResponses, &response)
			}

		}

	} else { //메콜 고려 안해도 돼서 없어도 됨
		fmt.Println("====== do not delete ======")
		physicalDevsMap := make(map[string]bool)
		for _, req := range reqs.ContainerRequests {
			//fmt.Println("456")
			for _, id := range req.DevicesIDs {
				if !deviceExists(devs, id) {
					return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
				}

				// Convert virtual GPUDeviceId to physical GPUDeviceID
				physicalDevId := getPhysicalDeviceID(id)
				//physicalDevId = "GPU-f6db4146-092d-146f-0814-8ff90b04f3d2"
				if !physicalDevsMap[physicalDevId] {
					physicalDevsMap[physicalDevId] = true
				}

				dev := getDeviceById(devs, id)
				if dev == nil {
					return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
				}

				if dev.Health != pluginapi.Healthy {
					return nil, fmt.Errorf("invalid allocation request with unhealthy device %s", id)
				}
			}

			// Set physical GPU devices as container visible devices
			visibleDevs := make([]string, 0, len(physicalDevsMap))
			visibleDev := ""
			//fmt.Println("devmps size :", len(physicalDevsMap))
			if allocatenum == 0 {
				//for visibleDev := range physicalDevsMap { //이게아니라 요청gpu 수만큼 올려야할듯?
				for i := 0; i < len(req.DevicesIDs); i++ {
					device, err := nvml.NewDevice(uint(i))
					if err != nil {
						fmt.Printf("device error\n")
					}
					visibleDev = device.UUID // 여기가 gpu 읽는곳으로 써야함
					fmt.Printf("Metric Collector using GPU UUID : %v\n", visibleDev)
					visibleDevs = append(visibleDevs, visibleDev)
				}
			} else {
				for i := 0; i < len(req.DevicesIDs); i++ {
					visibleDev = UUID[i]
					fmt.Printf("User Pod using GPU UUID : %v\n", visibleDev)
					visibleDevs = append(visibleDevs, visibleDev)
				}
			}

			response := pluginapi.ContainerAllocateResponse{
				Envs: map[string]string{
					"NVIDIA_VISIBLE_DEVICES": strings.Join(visibleDevs, ","),
				},
			}
			fmt.Println("Metric Collector Used GPU UUID : ", visibleDevs)

			// Set MPS environment variables - figure it out why it doesn't work?
			//response.Envs["CUDA_MPS_ACTIVE_THREAD_PERCENTAGE"] = fmt.Sprintf("%d", 100 * uint(len(req.DevicesIDs) / len(m.devs)))
			//response.Envs["CUDA_MPS_PIPE_DIRECTORY"] = "/tmp"
			//
			//mount := pluginapi.Mount{
			//	ContainerPath: "/tmp/nvidia-mps",
			//	HostPath: "/tmp/nvidia-mps",
			//}
			//response.Mounts = append(response.Mounts, &mount)

			responses.ContainerResponses = append(responses.ContainerResponses, &response)
		}
	}
	// loc, err := time.LoadLocation("Asia/Seoul")
	// if err != nil {
	// 	log.Fatalf("time error : %v", err)
	// }
	// t := time.Now().In(loc)
	// fmt.Println("Success Time : ", t)
	return &responses, nil
}

func (m *NvidiaDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	fmt.Println("prestartcontainer")
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *NvidiaDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Need to make sure all health check check against real device but not the virtual device

func (m *NvidiaDevicePlugin) healthcheck() {
	disableHealthChecks := strings.ToLower(os.Getenv(envDisableHealthChecks))
	if disableHealthChecks == "all" {
		disableHealthChecks = allHealthChecks
	}

	ctx, cancel := context.WithCancel(context.Background())

	var xids chan *pluginapi.Device
	if !strings.Contains(disableHealthChecks, "xids") {
		xids = make(chan *pluginapi.Device)
		go watchXIDs(ctx, m.devs, xids)
	}

	for {
		select {
		case <-m.stop:
			cancel()
			return
		case dev := <-xids:
			m.unhealthy(dev)
		}
	}
}

// Serve starts the gRPC server and register the device plugin to Kubelet
func (m *NvidiaDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		log.Printf("Could not start device plugin: %s", err)
		return err
	}
	log.Println("Starting to serve on", m.socket)

	err = m.Register(pluginapi.KubeletSocket, resourceName)
	if err != nil {
		log.Printf("Could not register device plugin: %s", err)
		m.Stop()
		return err
	}
	log.Println("Registered device plugin with Kubelet")

	return nil
}

func getDeviceById(devices []*pluginapi.Device, deviceId string) *pluginapi.Device {
	for _, d := range devices {
		if d.ID == deviceId {
			return d
		}
	}

	return nil
}

func (m *NvidiaDevicePlugin) GetPreferredAllocation(ctx context.Context, r *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	response := &pluginapi.PreferredAllocationResponse{}
	for _, req := range r.ContainerRequests {
		available, err := gpuallocator.NewDevicesFrom(req.AvailableDeviceIDs)
		if err != nil {
			return nil, fmt.Errorf("Unable to retrieve list of available devices: %v", err)
		}

		required, err := gpuallocator.NewDevicesFrom(req.MustIncludeDeviceIDs)
		if err != nil {
			return nil, fmt.Errorf("Unable to retrieve list of required devices: %v", err)
		}

		allocated := m.allocatePolicy.Allocate(available, required, int(req.AllocationSize))

		var deviceIds []string
		for _, device := range allocated {
			deviceIds = append(deviceIds, device.UUID)
		}

		resp := &pluginapi.ContainerPreferredAllocationResponse{
			DeviceIDs: deviceIds,
		}

		response.ContainerResponses = append(response.ContainerResponses, resp)
	}
	return response, nil
}
