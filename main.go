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
	up bool

	nomPower             float64
	batteryChargePercent float64

	timeOnBattery    time.Duration
	timeLeft         time.Duration
	minTimeLeft      time.Duration
	cumTimeOnBattery time.Duration

	loadPercent             float64
	minBatteryChargePercent float64

	batteryVoltage    float64
	lineVoltage       float64
	nomBatteryVoltage float64
	nomInputVoltage   float64
	hiTransVoltage    float64
	loTransVoltage    float64

	hostname string
	upsName  string
}

var (
	nominalPower = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_nominal_power",
		Help: "Nominal UPS Power",
	},
		[]string{"hostname", "upsname"},
	)

	batteryChargePercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_battery_charge_percent",
		Help: "Percentage Battery Charge",
	},
		[]string{"hostname", "upsname"},
	)

	loadPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_load_percent",
		Help: "Percentage Battery Load",
	},
		[]string{"hostname", "upsname"},
	)

	timeOnBattery = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_time_on_battery",
		Help: "Total time on UPS battery",
	},
		[]string{"hostname", "upsname"},
	)

	timeLeft = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_time_left",
		Help: "Time on UPS battery",
	},
		[]string{"hostname", "upsname"},
	)

	minTimeLeft = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_min_time_left",
		Help: "Time on UPS battery",
	},
		[]string{"hostname", "upsname"},
	)

	cumTimeOnBattery = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_cum_time_on_battery",
		Help: "Cumululative Time on UPS battery",
	},
		[]string{"hostname", "upsname"},
	)

	minBatteryChargePercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_min_battery_charge_percent",
		Help: "Minimum Battery Charge Percent",
	},
		[]string{"hostname", "upsname"},
	)

	batteryVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_battery_voltage",
		Help: "UPS Battery Voltage",
	},
		[]string{"hostname", "upsname"},
	)

	lineVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_line_voltage",
		Help: "UPS Line Voltage",
	},
		[]string{"hostname", "upsname"},
	)

	nomBatteryVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_nom_battery_votage",
		Help: "UPS Nominal Battery Voltage",
	},
		[]string{"hostname", "upsname"},
	)

	nomInputVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_nom_input_votage",
		Help: "UPS Nominal Input Voltage",
	},
		[]string{"hostname", "upsname"},
	)

	hiTransVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_hi_trans_voltage",
		Help: "UPS High Transimission Voltage",
	},
		[]string{"hostname", "upsname"},
	)

	loTransVoltage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ups_lo_trans_voltage",
		Help: "UPS Low Transimission Voltage",
	},
		[]string{"hostname", "upsname"},
	)
)

func main() {
	addr := flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	upsAddr := flag.String("ups-address", "", "The address of the acupsd daemon to query.")
	flag.Parse()

	log.Printf("Connection to UPS at: %s", *upsAddr)
	log.Printf("Metric listener at: %s", *addr)

	prometheus.MustRegister(nominalPower)
	prometheus.MustRegister(batteryChargePercent)
	prometheus.MustRegister(timeOnBattery)
	prometheus.MustRegister(timeLeft)
	prometheus.MustRegister(minTimeLeft)
	prometheus.MustRegister(cumTimeOnBattery)
	prometheus.MustRegister(loadPercent)
	prometheus.MustRegister(minBatteryChargePercent)
	prometheus.MustRegister(batteryVoltage)
	prometheus.MustRegister(lineVoltage)
	prometheus.MustRegister(nomBatteryVoltage)
	prometheus.MustRegister(nomInputVoltage)
	prometheus.MustRegister(hiTransVoltage)
	prometheus.MustRegister(loTransVoltage)

	go func() {
		c := time.Tick(10 * time.Second)
		for _ = range c {

			data, err := retrieveData(*upsAddr)
			if err != nil {
				log.Printf("Error: %+v", err)
			} else {
				fmt.Printf("%+v", data)
			}

			info, err := transformData(data)
			if err != nil {
				log.Printf("Error converting Data: %+v", err)
			}

			nominalPower.WithLabelValues(info.hostname, info.upsName).Set(info.nomPower)

			batteryChargePercent.WithLabelValues(info.hostname, info.upsName).Set(info.batteryChargePercent)
			timeOnBattery.WithLabelValues(info.hostname, info.upsName).Set(info.timeOnBattery.Seconds())

			timeLeft.WithLabelValues(info.hostname, info.upsName).Set(info.timeLeft.Seconds())
			minTimeLeft.WithLabelValues(info.hostname, info.upsName).Set(info.minTimeLeft.Seconds())

			cumTimeOnBattery.WithLabelValues(info.hostname, info.upsName).Set(info.minTimeLeft.Seconds())
			loadPercent.WithLabelValues(info.hostname, info.upsName).Set(info.loadPercent)
			minBatteryChargePercent.WithLabelValues(info.hostname, info.upsName).Set(info.minBatteryChargePercent)
			batteryVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.batteryVoltage)
			lineVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.lineVoltage)
			nomBatteryVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.nomBatteryVoltage)
			nomInputVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.nomInputVoltage)
			hiTransVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.hiTransVoltage)
			loTransVoltage.WithLabelValues(info.hostname, info.upsName).Set(info.loTransVoltage)

		}

	}()

	http.Handle("/metrics", prometheus.Handler())
	http.ListenAndServe(*addr, nil)
}

func transformData(ups map[string]string) (*upsInfo, error) {

	upsInfo := &upsInfo{}

	if ups["STATUS"] == "ONLINE" {
		upsInfo.up = true
	}

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

	if time, err := parseTime(ups["MINTIMEL"]); err != nil {
		return nil, err
	} else {
		upsInfo.minTimeLeft = time
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

	if percent, err := parseUnits(ups["MBATTCHG"]); err != nil {
		return nil, err
	} else {
		upsInfo.minBatteryChargePercent = percent
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

	if volts, err := parseUnits(ups["HITRANS"]); err != nil {
		return nil, err
	} else {
		upsInfo.hiTransVoltage = volts
	}

	if volts, err := parseUnits(ups["LOTRANS"]); err != nil {
		return nil, err
	} else {
		upsInfo.loTransVoltage = volts
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
