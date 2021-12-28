package main

import (
	"device-plugin/pkg/gpu/nvidia"
	"flag"
	"fmt"
	"log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var (
	vGPU = flag.Int("ketimpsgpu", 10, "Number of virtual GPUs")
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
