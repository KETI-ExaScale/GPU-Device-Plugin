package main

var MaxInstance = map[string]int{"A100": 7, "A30": 4}

var A100DefaultInstance = miginstance{7, 7}

var A30DefaultInstance = miginstance{4, 4}

type miginstance struct {
	GPUInstance     int
	ComputeInstance int
}
