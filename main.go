package main

import (
	"device-plugin/pkg/gpu/nvidia"
	"flag"
	"fmt"
	"log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var (
	vGPU        = flag.Int("ketimpsgpu", 10, "Number of virtual GPUs")
	InstanceGPU = flag.Int("ketiinstancegpu", 4, "Number of GPU Instance")
)

const VOLTA_MAXIMUM_MPS_CLIENT = 48

func main() {
	// loc, _ := time.LoadLocation("Asia/Seoul")
	flag.Parse()
	if err := nvml.Init(); err != nvml.SUCCESS {
		log.Printf("Failed to initialize NVML: %v.", err)
	}
	defer func() { log.Println("Shutdown of NVML returned:", nvml.Shutdown()) }()

	log.Println("Fetching devices.")
	devicenum, _ := nvml.DeviceGetCount()
	for i := 0; i < devicenum; i++ {
		device, _ := nvml.DeviceGetHandleByIndex(i)
		device.SetComputeMode(3)
		fmt.Println("Set GPU ComputeMode by Index :", i)
		cur, pend, ret := device.GetMigMode()
		if ret != nvml.SUCCESS {
			fmt.Println("Not Support Device by index : ", i)
		} else if cur == 0 && pend == 0 {
			device.SetMigMode(nvml.DEVICE_MIG_ENABLE)
		}
		if cur == 1 {
			maxmig, ret := device.GetMaxMigDeviceCount()
			if ret != nvml.SUCCESS {
				fmt.Println("Can't Use Migmode")
			} else {
				for j := 0; j < maxmig; j++ {
					migdevice, ret := device.GetMigDeviceHandleByIndex(j)
					if ret != nvml.SUCCESS {
						fmt.Println("no mig device by index : ", j)
					} else {
						id, _ := migdevice.GetGpuInstanceId()
						GPUInstance, _ := migdevice.GetGpuInstanceById(id)
						GPUInstance.Destroy()
					}
				}
			}
			if ret == nvml.SUCCESS {
				device.CreateGpuInstance(&nvml.GpuInstanceProfileInfo{Id: 19})
			}
		}
	}
	log.Println("Start KETI GPU device plugin")
	// fmt.Println(time.Now().In(loc))

	if *vGPU > VOLTA_MAXIMUM_MPS_CLIENT {
		log.Fatal("Number of virtual GPUs can not exceed maximum number of MPS clients")
	}

	vgm := nvidia.NewVirtualGPUManager(*vGPU)

	err := vgm.Run()
	if err != nil {
		log.Fatalf("Failed due to %v", err)
	}
}
