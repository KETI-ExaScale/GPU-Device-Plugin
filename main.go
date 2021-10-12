package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"device-plugin/pkg/gpu/nvidia"
)

var (
	vGPU = flag.Int("ketimpsgpu", 10, "Number of virtual GPUs")
)

const VOLTA_MAXIMUM_MPS_CLIENT = 48

func main() {
	loc, _ := time.LoadLocation("Asia/Seoul")
	flag.Parse()
	log.Println("Start virtual GPU device plugin")
	fmt.Println(time.Now().In(loc))

	if *vGPU > VOLTA_MAXIMUM_MPS_CLIENT {
		log.Fatal("Number of virtual GPUs can not exceed maximum number of MPS clients")
	}

	vgm := nvidia.NewVirtualGPUManager(*vGPU)

	err := vgm.Run()
	if err != nil {
		log.Fatalf("Failed due to %v", err)
	}
	fmt.Println("29")
}
