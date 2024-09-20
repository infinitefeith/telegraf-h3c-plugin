#!/bin/bash

telegraf_dir=$1
h3cplugin_dir=$(pwd)
rm -rf $telegraf_dir/plugins/inputs/h3c_telemetry_mdt
rm -rf $telegraf_dir/plugins/parsers/h3c_telemetry.go

cp -r $h3cplugin_dir/h3c_telemetry_mdt  $telegraf_dir/plugins/inputs
cp $h3cplugin_dir/h3c_telemetry_mdt.go $telegraf_dir/plugins/inputs/all
