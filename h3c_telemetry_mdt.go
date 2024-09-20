//go:build !custom || parsers || parsers.xpath

package all

import _ "github.com/influxdata/telegraf/plugins/inputs/h3c_telemetry_mdt" // register plugin
