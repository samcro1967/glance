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
