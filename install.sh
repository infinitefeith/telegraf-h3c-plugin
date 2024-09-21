#!/bin/bash

telegraf_dir=$1
h3cplugin_dir=$(pwd)

cat >> $telegraf_dir/plugins/inputs/all/h3c_telemetry_mdt.go << EOF
//go:build !custom || parsers || parsers.xpath

package all

import _ "github.com/influxdata/telegraf/plugins/inputs/h3c_telemetry_mdt" // register plugin
EOF

cp -r $h3cplugin_dir/h3c_telemetry_mdt  $telegraf_dir/plugins/inputs
