# apcups-exporter
Prometheus Exporter for APC UPS hardware via apcupsd

## UPS Status

The exporter lists two different types of status metric to be as flexible as possible.

1. `apc_status` has a label value of "status" which includes all the possible apcupsd status values, currently this results in the following for an "online" UPS:

```
# HELP apcups_status Current status of UPS
# TYPE apcups_status gauge
apcups_status{hostname="beaker.murf.org",status="boost",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="commlost",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="lowbatt",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="nobatt",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="onbatt",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="online",upsname="backups-950"} 1
apcups_status{hostname="beaker.murf.org",status="overload",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="replacebatt",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="shutting down",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="slave",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="slavedown",upsname="backups-950"} 0
apcups_status{hostname="beaker.murf.org",status="trim",upsname="backups-950"} 0
```

2. `apc_status_numeric` is a single metric with value as per the following status table
| status        | value |
|---------------|-------|
| online        | 0     |
| trim          | 1     |
| boost         | 2     |
| onbatt        | 3     |
| overload      | 4     |
| lowbatt       | 5     |
| replacebatt   | 6     |
| nobatt        | 7     |
| slave         | 8     |
| slavedown     | 9     |
| commlost      | 10    |
| shutting down | 11    |


