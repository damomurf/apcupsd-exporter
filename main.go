package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// map[VERSION:3.14.10 (13 September 2011) debian MINTIMEL:3 Minutes BATTDATE:2014-10-21 END APC:2016-08-30 17 NUMXFERS:0 NOMPOWER:480 Watts NOMINV:230 Volts FIRMWARE:925.T1 .I USB FW APC:001,036,0923 STATUS:ONLINE BCHARGE:100.0 Percent TONBATT:0 seconds HOSTNAME:beaker.murf.org CABLE:USB Cable TIMELEFT:104.6 Minutes SELFTEST:NO ALARMDEL:30 seconds STATFLAG:0x07000008 Status Flag DATE:2016-08-30 17 UPSMODE:Stand Alone MAXTIME:0 Seconds SENSE:Medium HITRANS:280.0 Volts LASTXFER:Unacceptable line voltage changes XOFFBATT:N/A SERIALNO:3B1443X05291 UPSNAME:backups-950 DRIVER:USB UPS Driver STARTTIME:2016-08-30 16 LOADPCT:5.0 Percent Load Capacity MBATTCHG:5 Percent LOTRANS:155.0 Volts BATTV:13.5 Volts CUMONBATT:0 seconds MODEL:Back-UPS XS 950U LINEV:242.0 Volts NOMBATTV:12.0 Volts

type upsInfo struct {
	status string

	nomPower             float64
	batteryChargePercent float64

	timeOnBattery    time.Duration
	timeLeft         time.Duration
	cumTimeOnBattery time.Duration

	loadPercent float64

	batteryVoltage    float64
	lineVoltage       float64
	nomBatteryVoltage float64
	nomInputVoltage   float64

	hostname string
	upsName  string
}

// See SVN code at https://sourceforge.net/p/apcupsd/svn/HEAD/tree/trunk/src/lib/apcstatus.c#l166 for
// list of statuses.
var statusList = []string{
	"online",
	"onbatt",
	"trim",
	"boost",
	"overload",
	"lowbatt",
	"replacebatt",
	"nobatt",
	"slave",
	"slavedown",
	"commlost",
	"shutting down",
}

var (
	labels = []string{"hostname", "upsname"}

	status = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_status",
		Help: "Current status of UPS",
	},
		append(labels, "status"),
	)

	statusNumeric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_status_numeric",
		Help: "Current status of UPS represented as integer",
	},
		labels,
	)

	nominalPower = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_nominal_power_watts",
		Help: "Nominal UPS Power",
	},
		labels,
	)

	batteryChargePercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_battery_charge_percent",
		Help: "Percentage Battery Charge",
	},
		labels,
	)

	loadPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_load_percent",
		Help: "Percentage Battery Load",
	},
		labels,
	)

	timeOnBattery = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_time_on_battery_seconds",
		Help: "Total time on UPS battery",
	},
		labels,
	)

	timeLeft = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_time_left_seconds",
		Help: "Time on UPS battery",
	},
		labels,
	)

	cumTimeOnBattery = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_cum_time_on_battery_seconds",
		Help: "Cumululative Time on UPS battery",
	},
		labels,
	)

	batteryVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_battery_volts",
		Help: "UPS Battery Voltage",
	},
		labels,
	)

	lineVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_line_volts",
		Help: "UPS Line Voltage",
	},
		labels,
	)

	nomBatteryVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_nom_battery_volts",
		Help: "UPS Nominal Battery Voltage",
	},
		labels,
	)

	nomInputVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_nom_input_volts",
		Help: "UPS Nominal Input Voltage",
	},
		labels,
	)

	collectSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "apcups_collect_time_seconds",
		Help: "Time to collect stats for last poll of UPS network interface",
	},
		labels,
	)
)

func main() {

	// TODO: Register a port for listening here: https://github.com/prometheus/prometheus/wiki/Default-port-allocations
	addr := flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	upsAddr := flag.String("ups-address", "localhost:3551", "The address of the acupsd daemon to query: hostname:port")
	flag.Parse()

	log.Printf("Connection to UPS at: %s", *upsAddr)
	log.Printf("Metric listener at: %s", *addr)

	prometheus.MustRegister(status)
	prometheus.MustRegister(statusNumeric)
	prometheus.MustRegister(nominalPower)
	prometheus.MustRegister(batteryChargePercent)
	prometheus.MustRegister(timeOnBattery)
	prometheus.MustRegister(timeLeft)
	prometheus.MustRegister(cumTimeOnBattery)
	prometheus.MustRegister(loadPercent)
	prometheus.MustRegister(batteryVoltage)
	prometheus.MustRegister(lineVoltage)
	prometheus.MustRegister(nomBatteryVoltage)
	prometheus.MustRegister(nomInputVoltage)
	prometheus.MustRegister(collectSeconds)

	go func() {
		c := time.Tick(10 * time.Second)
		for _ = range c {
			if err := collectUPSData(upsAddr); err != nil {
				log.Printf("Error collecting UPS data: %+v", err)
			}
		}

	}()

	http.Handle("/metrics", prometheus.Handler())
	http.ListenAndServe(*addr, nil)
}

func collectUPSData(upsAddr *string) error {

	gatherStart := time.Now()

	data, err := retrieveData(*upsAddr)
	if err != nil {
		return err
	}

	gatherDuration := time.Now().Sub(gatherStart)

	info, err := transformData(data)
	if err != nil {
		return err
	}
	collectSeconds.WithLabelValues(info.hostname, info.upsName).Set(gatherDuration.Seconds())

	log.Printf("%+v", info)

	for i, stat := range statusList {
		if stat == info.status {
			status.WithLabelValues(info.hostname, info.upsName, stat).Set(1)
			statusNumeric.WithLabelValues(info.hostname, info.upsName).Set(float64(i))
		} else {
			status.WithLabelValues(info.hostname, info.upsName, stat).Set(0)
		}
	}

	status.WithLabelValues(info.hostname, info.upsName, info.status).Set(1)

	nominalPower.WithLabelValues(info.hostname, info.upsName).Set(info.nomPower)

	batteryChargePercent.WithLabelValues(info.hostname, info.upsName).Set(info.batteryChargePercent)
	timeOnBattery.WithLabelValues(info.hostname, info.upsName).Set(info.timeOnBattery.Seconds())

	timeLeft.WithLabelValues(info.hostname, info.upsName).Set(info.timeLeft.Seconds())

	cumTimeOnBattery.WithLabelValues(info.hostname, info.upsName).Set(info.cumTimeOnBattery.Seconds())
	loadPercent.WithLabelValues(info.hostname, info.upsName).Set(info.loadPercent)
	batteryVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.batteryVoltage)
	lineVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.lineVoltage)
	nomBatteryVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.nomBatteryVoltage)
	nomInputVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.nomInputVoltage)

	return nil
}

func transformData(ups map[string]string) (*upsInfo, error) {

	upsInfo := &upsInfo{}

	upsInfo.status = strings.ToLower(ups["STATUS"])

	if nomPower, err := parseUnits(ups["NOMPOWER"]); err != nil {
		return nil, err
	} else {
		upsInfo.nomPower = nomPower
	}

	if chargePercent, err := parseUnits(ups["BCHARGE"]); err != nil {
		return nil, err
	} else {
		upsInfo.batteryChargePercent = chargePercent
	}

	if time, err := parseTime(ups["TONBATT"]); err != nil {
		return nil, err
	} else {
		upsInfo.timeOnBattery = time
	}

	if time, err := parseTime(ups["TIMELEFT"]); err != nil {
		return nil, err
	} else {
		upsInfo.timeLeft = time
	}

	if time, err := parseTime(ups["CUMONBATT"]); err != nil {
		return nil, err
	} else {
		upsInfo.cumTimeOnBattery = time
	}

	if percent, err := parseUnits(ups["LOADPCT"]); err != nil {
		return nil, err
	} else {
		upsInfo.loadPercent = percent
	}

	if volts, err := parseUnits(ups["BATTV"]); err != nil {
		return nil, err
	} else {
		upsInfo.batteryVoltage = volts
	}

	if volts, err := parseUnits(ups["LINEV"]); err != nil {
		return nil, err
	} else {
		upsInfo.lineVoltage = volts
	}

	if volts, err := parseUnits(ups["NOMBATTV"]); err != nil {
		return nil, err
	} else {
		upsInfo.nomBatteryVoltage = volts
	}

	if volts, err := parseUnits(ups["NOMINV"]); err != nil {
		return nil, err
	} else {
		upsInfo.nomInputVoltage = volts
	}

	upsInfo.hostname = ups["HOSTNAME"]
	upsInfo.upsName = ups["UPSNAME"]

	return upsInfo, nil
}

// parse time strings like 30 seconds or 1.25 minutes
func parseTime(t string) (time.Duration, error) {
	chunks := strings.Split(t, " ")
	fmtStr := chunks[0] + string(strings.ToLower(chunks[1])[0])
	return time.ParseDuration(fmtStr)
}

// parse generic units, splitting of units name and converting to float
func parseUnits(v string) (float64, error) {
	if v == ""{
		return 0, nil
	}
	return strconv.ParseFloat(strings.Split(v, " ")[0], 32)
}

func retrieveData(hostPort string) (map[string]string, error) {

	conn, err := net.Dial("tcp", hostPort)
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to remote port: %+v", err)
	}

	if _, err = conn.Write([]byte{0, 6}); err != nil {
		return nil, fmt.Errorf("Error writing command length: %+v", err)
	}

	if _, err = conn.Write([]byte("status")); err != nil {
		return nil, fmt.Errorf("Error writing command data: %+v", err)
	}

	complete := false
	upsData := map[string]string{}

	for !complete {
		sizeBuf := []byte{0, 0}
		var size int16
		if _, err := conn.Read(sizeBuf); err != nil {
			return nil, fmt.Errorf("Error reading size from incoming reader: %+v", err)
		}

		if err = binary.Read(bytes.NewBuffer(sizeBuf), binary.BigEndian, &size); err != nil {
			return nil, fmt.Errorf("Error decoding size in response: %+v", err)
		}

		if size > 0 {
			data := make([]byte, size)
			if _, err = conn.Read(data); err != nil {
				log.Panicf("Error reading size from incoming reader: %+v", err)
			}
			chunks := strings.Split(string(data), ":")
			upsData[strings.TrimSpace(chunks[0])] = strings.TrimSpace(chunks[1])

		} else {
			complete = true
		}
	}

	if err = conn.Close(); err != nil {
		log.Panicf("Error closing apcupsd connection: %+v", err)
	}

	return upsData, nil

}
