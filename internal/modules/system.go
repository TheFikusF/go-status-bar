package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type cpuSampler struct {
	lastIdle  uint64
	lastTotal uint64
	ready     bool
}

func NewCPU() gtk.Widgetter {
	module := newTextModule("cpu")
	sampler := &cpuSampler{}
	startPolling(500*time.Millisecond, func() {
		ui(func() { setTextModule(module, sampler.read()) })
	})
	return module.Box
}

func (c *cpuSampler) read() string {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return ""
	}

	fields := strings.Fields(strings.SplitN(string(stat), "\n", 2)[0])
	if len(fields) < 5 {
		return ""
	}

	var total uint64
	values := make([]uint64, 0, len(fields)-1)
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return ""
		}
		values = append(values, value)
		total += value
	}

	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}

	if !c.ready {
		c.lastIdle, c.lastTotal, c.ready = idle, total, true
		return "--% "
	}

	totalDelta := total - c.lastTotal
	idleDelta := idle - c.lastIdle
	c.lastIdle, c.lastTotal = idle, total
	if totalDelta == 0 {
		return "--% "
	}

	usage := 100 * float64(totalDelta-idleDelta) / float64(totalDelta)
	return fmt.Sprintf("%2.0f%% ", usage)
}

func NewMemory() gtk.Widgetter {
	module := newTextModule("memory")
	startPolling(3*time.Second, func() {
		ui(func() { setTextModule(module, readMemory()) })
	})
	return module.Box
}

func readMemory() string {
	meminfo, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return ""
	}

	values := map[string]uint64{}
	for _, line := range strings.Split(string(meminfo), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value
		}
	}

	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 {
		return ""
	}

	used := 100 * float64(total-available) / float64(total)
	return fmt.Sprintf("%2.0f%% ", used)
}

func NewTemperature() gtk.Widgetter {
	module := newTextModule("temperature")
	startPolling(5*time.Second, func() {
		temp := readTemperature()
		ui(func() {
			setTextModule(module, temp)
			if strings.Contains(temp, "HOT") {
				module.Box.AddCSSClass("critical")
			} else {
				module.Box.RemoveCSSClass("critical")
			}
		})
	})
	return module.Box
}

func readTemperature() string {
	entries, err := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	if err != nil || len(entries) == 0 {
		return ""
	}

	maxC := 0.0
	for _, entry := range entries {
		data, err := os.ReadFile(entry)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		celsius := value / 1000.0
		if celsius > maxC {
			maxC = celsius
		}
	}

	if maxC == 0 {
		return ""
	}
	if maxC >= 80 {
		return fmt.Sprintf("%2.0f°C ", maxC)
	}
	return fmt.Sprintf("%2.0f°C ", maxC)
}
