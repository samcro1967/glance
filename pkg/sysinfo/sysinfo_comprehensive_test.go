package sysinfo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/sensors"
)

func TestTimestampJSONRoundTrip(t *testing.T) {
	original := timestampJSON{Time: time.Unix(1788116400, 0)}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "1788116400" {
		t.Fatalf("json=%s", data)
	}
	var decoded timestampJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Equal(original.Time) {
		t.Fatalf("decoded=%v", decoded.Time)
	}
	if err := json.Unmarshal([]byte(`"bad"`), &decoded); err == nil {
		t.Fatal("expected invalid timestamp error")
	}
}

func TestInferCPUTempSensorKnownKeys(t *testing.T) {
	keys := []string{"coretemp_package_id_0", "coretemp", "k10temp", "zenpower", "cpu_thermal"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			readings := []sensors.TemperatureStat{{SensorKey: "other", Temperature: 10}, {SensorKey: key, Temperature: 55}}
			got := inferCPUTempSensor(readings)
			if got == nil || got.SensorKey != key || got.Temperature != 55 {
				t.Fatalf("got=%#v", got)
			}
		})
	}
	if got := inferCPUTempSensor([]sensors.TemperatureStat{{SensorKey: "gpu", Temperature: 60}}); got != nil {
		t.Fatalf("unexpected sensor=%#v", got)
	}
	if inferCPUTempSensor(nil) != nil {
		t.Fatal("nil readings should return nil")
	}
}

func TestCollectReturnsInitializedResult(t *testing.T) {
	info, errs := Collect(&SystemInfoRequest{HideMountpointsByDefault: true})
	if info == nil {
		t.Fatal("Collect returned nil info")
	}
	if info.Mountpoints == nil {
		t.Fatal("mountpoints should be initialized")
	}
	for _, mp := range info.Mountpoints {
		if mp.UsedPercent > 100 {
			t.Fatalf("mountpoint percent=%d", mp.UsedPercent)
		}
	}
	if info.CPU.Load1Percent > 100 || info.CPU.Load15Percent > 100 || info.Memory.UsedPercent > 100 || info.Memory.SwapUsedPercent > 100 {
		t.Fatal("percentage exceeded 100")
	}
	_ = errs // platform-dependent diagnostics are valid and intentionally not asserted.
}

func boolPointer(value bool) *bool {
	return &value
}

func TestSystemInfoRequestFilterMountpoints(t *testing.T) {
	info := &SystemInfo{
		Mountpoints: []MountpointInfo{
			{Path: "/", Name: "/", UsedPercent: 40},
			{Path: "/mnt/data", Name: "/mnt/data", UsedPercent: 80},
			{Path: "/boot", Name: "/boot", UsedPercent: 20},
		},
	}

	request := &SystemInfoRequest{
		HideMountpointsByDefault: true,
		Mountpoints: map[string]MointpointRequest{
			"/": {
				Hide: boolPointer(false),
			},
			"/mnt/data": {
				Name: "Data",
				Hide: boolPointer(false),
			},
			"/boot": {
				Hide: boolPointer(true),
			},
		},
	}

	request.Filter(info)

	if len(info.Mountpoints) != 2 {
		t.Fatalf("got %d mountpoints, want 2: %#v", len(info.Mountpoints), info.Mountpoints)
	}

	if info.Mountpoints[0].Path != "/mnt/data" {
		t.Fatalf("first mountpoint path = %q, want %q", info.Mountpoints[0].Path, "/mnt/data")
	}

	if info.Mountpoints[0].Name != "Data" {
		t.Fatalf("first mountpoint name = %q, want %q", info.Mountpoints[0].Name, "Data")
	}

	if info.Mountpoints[1].Path != "/" {
		t.Fatalf("second mountpoint path = %q, want %q", info.Mountpoints[1].Path, "/")
	}
}

func TestSystemInfoRequestFilterExplicitHide(t *testing.T) {
	info := &SystemInfo{
		Mountpoints: []MountpointInfo{
			{Path: "/", Name: "/", UsedPercent: 40},
			{Path: "/mnt/data", Name: "/mnt/data", UsedPercent: 80},
		},
	}

	request := &SystemInfoRequest{
		Mountpoints: map[string]MointpointRequest{
			"/mnt/data": {
				Hide: boolPointer(true),
			},
		},
	}

	request.Filter(info)

	if len(info.Mountpoints) != 1 {
		t.Fatalf("got %d mountpoints, want 1: %#v", len(info.Mountpoints), info.Mountpoints)
	}

	if info.Mountpoints[0].Path != "/" {
		t.Fatalf("remaining mountpoint = %q, want %q", info.Mountpoints[0].Path, "/")
	}
}

func TestSystemInfoRequestFilterNilInputs(t *testing.T) {
	var request *SystemInfoRequest
	request.Filter(&SystemInfo{})

	request = &SystemInfoRequest{}
	request.Filter(nil)
}
