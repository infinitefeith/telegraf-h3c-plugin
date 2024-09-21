package h3c_telemetry_mdt

import (
	"testing"

	telemetry "github.com/infinitefeith//telegraf-h3c-plugin/h3c_telemetry_mdt/telemetry_proto"

	"github.com/influxdata/telegraf/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestHandleTelemetryJson(t *testing.T) {
	c := H3cTelemetryMDT{
		Log:       testutil.Logger{},
		Transport: "dummy",
	}
	acc := &testutil.Accumulator{}
	err := c.Start()
	// error is expected since we are passing in dummy transport
	require.Error(t, err)

	telem := &telemetry.Telemetry{
		MsgTimestamp:        1294792357260,
		ProducerName:        "H3C",
		NodeIdStr:           "LEAF1",
		ProductName:         "H3C S6530X-48Y8C",
		CollectionStartTime: 1294792357260,
		CollectionEndTime:   1294792357260,
		SensorPath:          "NETANALYSIS/UnifiedFlowMonitorEvent",
		ExceptDesc:          "OK",
		Encoding:            telemetry.Telemetry_Encoding_JSON,
		DataStr:             "{\"UnifiedFlowMonitorEvent\":{\"UnifiedFlowMonitorEntry\":[{\"ChassisID\":0,\"SlotID\":1,\"SourceIP\":\"30.1.1.2\",\"SourceMask\":32,\"DestinationIP\":\"20.1.1.1\",\"DestinationMask\":32,\"Protocol\":132,\"SourcePort\":3001,\"DestinationPort\":3002,\"StartTimestampSec\":1294787950,\"StartTimestampNanoSec\":859000000,\"LastTimestampSec\":1294792357,\"LastTimestampNanoSec\":127855377,\"PakcetCount\":6158,\"ByteCount\":788224},{\"ChassisID\":0,\"SlotID\":1,\"SourceIP\":\"30.1.1.2\",\"SourceMask\":32,\"DestinationIP\":\"20.1.1.1\",\"DestinationMask\":32,\"Protocol\":132,\"SourcePort\":3001,\"DestinationPort\":3002,\"StartTimestampSec\":1294787950,\"StartTimestampNanoSec\":859000000,\"LastTimestampSec\":1294792357,\"LastTimestampNanoSec\":135145781,\"PakcetCount\":6157,\"ByteCount\":788096},{\"ChassisID\":0,\"SlotID\":1,\"SourceIP\":\"30.1.1.2\",\"SourceMask\":32,\"DestinationIP\":\"20.1.1.1\",\"DestinationMask\":32,\"Protocol\":132,\"SourcePort\":3001,\"DestinationPort\":3002,\"StartTimestampSec\":1294787950,\"StartTimestampNanoSec\":859000000,\"LastTimestampSec\":1294792357,\"LastTimestampNanoSec\":142436160,\"PakcetCount\":6158,\"ByteCount\":788224},{\"ChassisID\":0,\"SlotID\":1,\"SourceIP\":\"30.1.1.2\",\"SourceMask\":32,\"DestinationIP\":\"20.1.1.1\",\"DestinationMask\":32,\"Protocol\":132,\"SourcePort\":3001,\"DestinationPort\":3002,\"StartTimestampSec\":1294787950,\"StartTimestampNanoSec\":859000000,\"LastTimestampSec\":1294792357,\"LastTimestampNanoSec\":149726550,\"PakcetCount\":6157,\"ByteCount\":788096},{\"ChassisID\":0,\"SlotID\":1,\"SourceIP\":\"30.1.1.2\",\"SourceMask\":32,\"DestinationIP\":\"20.1.1.1\",\"DestinationMask\":32,\"Protocol\":132,\"SourcePort\":3001,\"DestinationPort\":3002,\"StartTimestampSec\":1294787950,\"StartTimestampNanoSec\":859000000,\"LastTimestampSec\":1294792357,\"LastTimestampNanoSec\":157016937,\"PakcetCount\":6158,\"ByteCount\":788224},{\"ChassisID\":0,\"SlotID\":1,\"SourceIP\":\"30.1.1.2\",\"SourceMask\":32,\"DestinationIP\":\"20.1.1.1\",\"DestinationMask\":32,\"Protocol\":132,\"SourcePort\":3001,\"DestinationPort\":3002,\"StartTimestampSec\":1294787950,\"StartTimestampNanoSec\":859000000,\"LastTimestampSec\":1294792357,\"LastTimestampNanoSec\":164307337,\"PakcetCount\":6158,\"ByteCount\":788224},{\"ChassisID\":0,\"SlotID\":1,\"SourceIP\":\"30.1.1.2\",\"SourceMask\":32,\"DestinationIP\":\"20.1.1.1\",\"DestinationMask\":32,\"Protocol\":132,\"SourcePort\":3001,\"DestinationPort\":3002,\"StartTimestampSec\":1294787950,\"StartTimestampNanoSec\":859000000,\"LastTimestampSec\":1294792353,\"LastTimestampNanoSec\":175859815,\"PakcetCount\":6157,\"ByteCount\":788096}]}}",
	}

	data, err := proto.Marshal(telem)
	require.EqualError(t, err)

	c.HandleTelemetry(data)
	require.Empty(t, acc.Errors)

	tags := map[string]interface{}{
		"producer_name": "H3C",
		"node_id_str":   "LEAF1",
		"product_name":  "H3C S6530X-48Y8C",
		"sensor_path":   "NETANALYSIS/UnifiedFlowMonitorEvent",
		"except_desc":   "OK",
	}
	fields := map[string]interface{}{"NETANALYSIS/UnifiedFlowMonitorEvent/ChassisID": 0}
	acc.AssertContainsTaggedFields(t, "NETANALYSIS", fields, tags)
}
