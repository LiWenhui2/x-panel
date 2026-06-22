package system

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Gauge struct {
	Used  int64 `json:"used"`
	Total int64 `json:"total"`
}

type Status struct {
	CPUPercent float64 `json:"cpuPercent"`
	Memory     Gauge   `json:"memory"`
	Swap       Gauge   `json:"swap"`
	Disk       Gauge   `json:"disk"`
	Load1      float64 `json:"load1"`
	Load5      float64 `json:"load5"`
	Load15     float64 `json:"load15"`
	Uptime     int64   `json:"uptime"`
	UploadBPS   int64     `json:"uploadBps"`
	DownloadBPS int64     `json:"downloadBps"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	CollectedAt time.Time `json:"collectedAt"`
}

func Collect(ctx context.Context) Status {
	firstCPU := readCPU()
	firstNet := readNet()
	time.Sleep(300 * time.Millisecond)
	secondCPU := readCPU()
	secondNet := readNet()
	status := Status{OS: runtime.GOOS, Arch: runtime.GOARCH, CollectedAt: time.Now().UTC()}
	status.CPUPercent = cpuPercent(firstCPU, secondCPU)
	status.Memory, status.Swap = readMemory()
	status.Disk = readDisk(ctx)
	status.Load1, status.Load5, status.Load15 = readLoad()
	status.Uptime = readUptime()
	status.DownloadBPS = int64(float64(secondNet.rx-firstNet.rx) / 0.3)
	status.UploadBPS = int64(float64(secondNet.tx-firstNet.tx) / 0.3)
	if status.DownloadBPS < 0 { status.DownloadBPS = 0 }
	if status.UploadBPS < 0 { status.UploadBPS = 0 }
	return status
}

type cpuSample struct{ idle, total uint64 }
func readCPU() cpuSample {
	content, _ := os.ReadFile("/proc/stat")
	fields := strings.Fields(strings.Split(string(content), "\n")[0])
	var values []uint64
	for _, field := range fields[1:] { value, _ := strconv.ParseUint(field, 10, 64); values = append(values, value) }
	var total uint64
	for _, value := range values { total += value }
	idle := uint64(0); if len(values) > 3 { idle = values[3] }
	return cpuSample{idle: idle, total: total}
}
func cpuPercent(a, b cpuSample) float64 { if b.total <= a.total { return 0 }; total := b.total-a.total; idle := b.idle-a.idle; return float64(total-idle)*100/float64(total) }

func readMemory() (Gauge, Gauge) {
	content, _ := os.ReadFile("/proc/meminfo")
	values := map[string]int64{}
	for _, line := range strings.Split(string(content), "\n") { fields := strings.Fields(line); if len(fields) >= 2 { value, _ := strconv.ParseInt(fields[1], 10, 64); values[strings.TrimSuffix(fields[0], ":")] = value * 1024 } }
	memTotal, memAvailable := values["MemTotal"], values["MemAvailable"]
	swapTotal, swapFree := values["SwapTotal"], values["SwapFree"]
	return Gauge{Used: memTotal - memAvailable, Total: memTotal}, Gauge{Used: swapTotal - swapFree, Total: swapTotal}
}

func readDisk(ctx context.Context) Gauge {
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Second); defer cancel()
	out, err := exec.CommandContext(commandCtx, "df", "-B1", "/").Output(); if err != nil { return Gauge{} }
	lines := strings.Split(strings.TrimSpace(string(out)), "\n"); if len(lines) < 2 { return Gauge{} }
	fields := strings.Fields(lines[1]); if len(fields) < 4 { return Gauge{} }
	total, _ := strconv.ParseInt(fields[1], 10, 64); used, _ := strconv.ParseInt(fields[2], 10, 64)
	return Gauge{Used: used, Total: total}
}

func readLoad() (float64,float64,float64) { content,_:=os.ReadFile("/proc/loadavg"); f:=strings.Fields(string(content)); if len(f)<3{return 0,0,0}; a,_:=strconv.ParseFloat(f[0],64); b,_:=strconv.ParseFloat(f[1],64); c,_:=strconv.ParseFloat(f[2],64); return a,b,c }
func readUptime() int64 { content,_:=os.ReadFile("/proc/uptime"); f:=strings.Fields(string(content)); if len(f)==0{return 0}; v,_:=strconv.ParseFloat(f[0],64); return int64(v) }
type netSample struct{ rx, tx uint64 }
func readNet() netSample { content,_:=os.ReadFile("/proc/net/dev"); sample:=netSample{}; for _,line:=range strings.Split(string(content),"\n"){ if !strings.Contains(line,":"){continue}; parts:=strings.Split(line,":"); name:=strings.TrimSpace(parts[0]); if name=="lo"{continue}; fields:=strings.Fields(parts[1]); if len(fields)<16{continue}; rx,_:=strconv.ParseUint(fields[0],10,64); tx,_:=strconv.ParseUint(fields[8],10,64); sample.rx+=rx; sample.tx+=tx }; return sample }
