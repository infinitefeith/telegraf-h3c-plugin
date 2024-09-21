package h3c_telemetry_mdt

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	telemetry "github.com/infinitefeith/telegraf-h3c-plugin/h3c_telemetry_mdt/telemetry_proto"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/metric"
	internaltls "github.com/influxdata/telegraf/plugins/common/tls"
	"github.com/influxdata/telegraf/plugins/inputs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials" // Register GRPC gzip decoder to support compressed telemetry
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
)

const (
	// Maximum telemetry payload size (in bytes) to accept for GRPC dialout transport
	tcpMaxMsgLen uint32 = 0
)

// H3cTelemetryMDT plugin VRPs
type H3cTelemetryMDT struct {
	// Though unused in the code, required by protoc-gen-go-grpc to maintain compatibility
	telemetry.UnimplementedGRPCDialoutV3Server
	// Common configuration
	Transport      string
	ServiceAddress string            `toml:"service_address"`
	MaxMsgSize     int               `toml:"max_msg_size"`
	Aliases        map[string]string `toml:"aliases"`
	EmbeddedTags   []string          `toml:"embedded_tags"`
	Log            telegraf.Logger
	// GRPC TLS settings
	internaltls.ServerConfig

	// Internal listener / client handle
	grpcServer *grpc.Server
	listener   net.Listener

	// Internal state
	aliases   map[string]string
	warned    map[string]struct{}
	extraTags map[string]map[string]struct{}
	metrics   []telegraf.Metric
	mutex     sync.Mutex
	acc       telegraf.Accumulator
	wg        sync.WaitGroup
}

// Start the H3C Telemetry dialout service
func (c *H3cTelemetryMDT) Start(acc telegraf.Accumulator) error {
	var err error
	c.acc = acc
	c.listener, err = net.Listen("tcp", c.ServiceAddress)
	if err != nil {
		return err
	}
	switch c.Transport {
	case "grpc":
		// set tls and max size of packet
		var opts []grpc.ServerOption
		tlsConfig, err := c.ServerConfig.TLSConfig()

		if err != nil {
			c.listener.Close()
			return err
		} else if tlsConfig != nil {
			tlsConfig.ClientAuth = tls.RequestClientCert
			opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		}

		if c.MaxMsgSize > 0 {
			opts = append(opts, grpc.MaxRecvMsgSize(c.MaxMsgSize))
		}
		// create grpc server
		c.grpcServer = grpc.NewServer(opts...)
		// Register the server with the method that receives the data
		telemetry.RegisterGRPCDialoutV3Server(c.grpcServer, c)

		c.wg.Add(1)
		go func() {
			// Listen on the ServiceAddress port
			c.grpcServer.Serve(c.listener)
			c.wg.Done()
		}()

	default:
		c.listener.Close()
		return fmt.Errorf("invalid H3C transport: %s", c.Transport)
	}

	return nil
}

// AcceptTCPDialoutClients defines the TCP dialout server main routine
func (c *H3cTelemetryMDT) acceptTCPClientsddd() {
	// Keep track of all active connections, so we can close them if necessary
	var mutex sync.Mutex
	clients := make(map[net.Conn]struct{})

	for {
		conn, err := c.listener.Accept()
		if neterr, ok := err.(*net.OpError); ok && (neterr.Timeout() || neterr.Temporary()) {
			continue
		} else if err != nil {
			break // Stop() will close the connection so Accept() will fail here
		}

		mutex.Lock()
		clients[conn] = struct{}{}
		mutex.Unlock()

		// Individual client connection routine
		c.wg.Add(1)
		go func() {
			c.Log.Debugf("D! Accepted H3C MDT TCP dialout connection from %s", conn.RemoteAddr())
			if err := c.handleTCPClient(conn); err != nil {
				c.acc.AddError(err)
			}
			c.Log.Debugf("Closed H3C MDT TCP dialout connection from %s", conn.RemoteAddr())

			mutex.Lock()
			delete(clients, conn)
			mutex.Unlock()

			conn.Close()
			c.wg.Done()
		}()
	}

	// Close all remaining client connections
	mutex.Lock()
	for client := range clients {
		if err := client.Close(); err != nil {
			c.Log.Errorf("Failed to close TCP dialout client: %v", err)
		}
	}
	mutex.Unlock()
}

// Handle a TCP telemetry client
func (c *H3cTelemetryMDT) handleTCPClient(conn net.Conn) error {
	// TCP Dialout telemetry framing header
	var hdr struct {
		MsgType       uint16
		MsgEncap      uint16
		MsgHdrVersion uint16
		MsgFlags      uint16
		MsgLen        uint32
	}

	var payload bytes.Buffer

	for {
		// Read and validate dialout telemetry header
		if err := binary.Read(conn, binary.BigEndian, &hdr); err != nil {
			return err
		}

		maxMsgSize := tcpMaxMsgLen
		if c.MaxMsgSize > 0 {
			maxMsgSize = uint32(c.MaxMsgSize)
		}

		if hdr.MsgLen > maxMsgSize {
			return fmt.Errorf("dialout packet too long: %v", hdr.MsgLen)
		} else if hdr.MsgFlags != 0 {
			return fmt.Errorf("invalid dialout flags: %v", hdr.MsgFlags)
		}

		// Read and handle telemetry packet
		payload.Reset()
		if size, err := payload.ReadFrom(io.LimitReader(conn, int64(hdr.MsgLen))); size != int64(hdr.MsgLen) {
			if err != nil {
				return err
			}
			return fmt.Errorf("TCP dialout premature EOF")
		}
	}
}

// implement the rpc method of h3c-grpc-dialout.proto
func (c *H3cTelemetryMDT) DialoutV3(stream telemetry.GRPCDialoutV3_DialoutV3Server) error {
	peer, peerOK := peer.FromContext(stream.Context())
	if peerOK {
		c.Log.Debugf("Accepted H3C GRPC dialout connection from %s", peer.Addr)
	}

	//var chunkBuffer bytes.Buffer
	for {
		packet, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.acc.AddError(fmt.Errorf("GRPC dialout receive error: %s, %v", c.listener.Addr(), err))
			}
			break
		}

		if len(packet.Data) == 0 && len(packet.Errors) != 0 {
			c.acc.AddError(fmt.Errorf("GRPC dialout error: %s", packet.Errors))
			break
		}

		var metrics []telegraf.Metric
		var errParse error
		// grpc data
		if len(packet.GetData()) != 0 {
			// c.Log.Debugf("D! GRPC dialout data: %s", hex.EncodeToString(packet.GetData()))
			metrics, errParse = c.HandleTelemetry(packet.GetData())
			if errParse != nil {
				c.acc.AddError(errParse)
				c.stop()
				return fmt.Errorf("[input.h3c_telemetry_dialout] error when parse grpc stream %t", errParse)
			}
		}
		for _, metric := range metrics {
			c.acc.AddMetric(metric)
		}
	}
	if peerOK {
		c.Log.Debugf("D! Closed H3C GRPC dialout connection from %s", peer.Addr)
	}
	return nil
}

func (c *H3cTelemetryMDT) HandleTelemetry(buf []byte) ([]telegraf.Metric, error) {
	msg := &telemetry.Telemetry{}
	errParse := proto.Unmarshal(buf, msg)
	if errParse != nil {
		return nil, errParse
	}
	//c.Log.Debugf("telemetry header : %+v\n", msg)

	// trans telemetry header into map[string]interface{}
	headerMap, errToMap := protoMsgToMap(msg)
	if errToMap != nil {
		return nil, errToMap
	}

	switch msg.Encoding {
	case telemetry.Telemetry_Encoding_GPB:
		dataGpb := msg.GetDataGpb()
		if dataGpb == nil {
			c.Log.Errorf("E! [grpc ParseJsonGpb] get gpb data")
			return nil, nil
		}
		// get protoPath
		protoPath := msg.SensorPath
		rows := dataGpb.GetRow()
		var rowsInMaps []map[string]interface{}
		var rowMsgs []proto.Message
		// Service layer decoding
		for _, row := range rows {
			contentMsg, errGetType := GetTypeByProtoPath(protoPath, DEFAULT_VERSION)
			if errGetType != nil {
				c.Log.Errorf("E! [grpc ParseJsonGpb] get type according protoPath: %v", errGetType)
				return nil, errGetType
			}
			errDecode := proto.Unmarshal(row.Content, contentMsg)

			rowMap, errToMap := protoMsgToMap(contentMsg)
			if errToMap != nil {
				return nil, errToMap
			}
			rowMap[MsgTimeStampKeyName] = row.Timestamp
			rowsInMaps = append(rowsInMaps, rowMap)
			rowMsgs = append(rowMsgs, contentMsg)
			if errDecode != nil {
				return nil, errDecode
			}
		}
		c.debugLogGpb(msg, rowMsgs)
		metrics, err := c.flattenProtoMsg(headerMap, rowsInMaps, "")
		return metrics, err
	case telemetry.Telemetry_Encoding_JSON:
		// parse data firstly
		var msgMap map[string]interface{}
		errToMap := json.Unmarshal([]byte(msg.GetDataStr()), &msgMap)
		if errToMap != nil {
			return nil, fmt.Errorf("[grpc ParseJson] proto message decodec to map: %v", errToMap)
		}
		// parse
		var msgsInMaps []map[string]interface{}
		msgsInMaps = append(msgsInMaps, msgMap)
		delete(headerMap, JsonMsgKeyName)
		c.debuglogJson(msg)
		metrics, err := c.flattenProtoMsg(headerMap, msgsInMaps, "") // JsonMsgKeyName+KeySeperator+RowKeyName="data_str.json"
		return metrics, err
	default:
		c.Log.Errorf("E! [grpc parser]<encoding is not support>")
	}
	return nil, nil
}

func (c *H3cTelemetryMDT) debuglogJson(header *telemetry.Telemetry) {

	headerStr, err := json.MarshalIndent(header, "", " ")
	if err == nil {
		c.Log.Debugf("==================================== JSON start msg_timestamp: %v================================\n", header.MsgTimestamp)
		c.Log.Debugf("header is : \n%s", headerStr)
	} else {
		c.Log.Debugf("error when log header")
	}
	c.Log.Debugf("==================================== JSON end ================================\n")
}

func (c *H3cTelemetryMDT) debugLogGpb(header *telemetry.Telemetry, rows []proto.Message) {
	headerStr, err := json.MarshalIndent(header, "", " ")
	if err == nil {
		c.Log.Debugf("==================================== GPB start msg_timestamp: %v================================\n", header.MsgTimestamp)
		c.Log.Debugf("header is : \n%s", headerStr)
	} else {
		c.Log.Debugf("error when log header")
	}
	c.Log.Debugf("content is : \n")
	for _, row := range rows {
		rowStr, err := json.MarshalIndent(row, "", " ")
		if err == nil {
			c.Log.Debugf("%s", rowStr)
		} else {
			c.Log.Debugf("error when log rows")
		}
	}
	c.Log.Debugf("==================================== GPB end ================================\n")
}

func (c *H3cTelemetryMDT) ParseLine(line string) (telegraf.Metric, error) {
	panic("implement me")
}

func (c *H3cTelemetryMDT) SetDefaultTags(tags map[string]string) {
	panic("implement me")
}

func (c *H3cTelemetryMDT) flattenProtoMsg(telemetryHeader map[string]interface{}, rowsDecodec []map[string]interface{}, startFieldName string) ([]telegraf.Metric, error) {
	kvHeader := KVStruct{}
	errHeader := kvHeader.FullFlattenStruct("", telemetryHeader, true, true)
	if errHeader != nil {
		return nil, errHeader
	}

	//// debug start
	//c.Log.Debugf("D! -------------------------------------Header START-----------------------------------------\n")
	//for k, v := range kvHeader.Fields {
	//    c.Log.Debugf("D! k: %s, v: %v ", k, v)
	//}
	//c.Log.Debugf("D! ------------------------------------- Header END -----------------------------------------\n")
	//// debug end

	var metrics []telegraf.Metric
	// one row into one metric
	for _, rowDecodec := range rowsDecodec {
		kvWithRow := KVStruct{}
		errRows := kvWithRow.FullFlattenStruct(startFieldName, rowDecodec, true, true)
		if errRows != nil {
			return nil, errRows
		}
		fields, tm, errMerge := c.mergeMaps(kvHeader.Fields, kvWithRow.Fields)
		if errMerge != nil {
			return nil, errMerge
		}
		metric := metric.New(telemetryHeader[SensorPathKey].(string), nil, fields, tm)
		// if err != nil {
		// 	return nil, err
		// }
		// debug start
		// c.Log.Debugf("D! -------------------------------------Fields START time is %v-----------------------------------------\n", metric.Time())
		// for k, v := range metric.Fields() {
		//    c.Log.Debugf("k: %s, v: %v ", k, v)
		// }
		// c.Log.Debugf("D! ------------------------------------- Fields END -----------------------------------------\n")
		// debug end

		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func (c *H3cTelemetryMDT) mergeMaps(maps ...map[string]interface{}) (map[string]interface{}, time.Time, error) {
	res := make(map[string]interface{})
	timestamp := time.Time{}
	for _, m := range maps {
		for k, v := range m {
			if strings.HasSuffix(k, "_time") || strings.HasSuffix(k, MsgTimeStampKeyName) {
				timeStruct, timeStr, errCal := calTimeByStamp(v)
				if errCal != nil {
					return nil, time.Time{}, fmt.Errorf("E! [grpc parser] when calculate time, key name is %s, time is %t, error is %v", k, v, errCal)
				}
				if k == MsgTimeStampKeyName {
					timestamp = timeStruct
					c.Log.Debugf("D! the row timestamp is %s\n", timestamp.Format(TimeFormat))
					continue
				}
				if timeStr != "" {
					res[k] = timeStr
					continue
				}
			}
			res[k] = v
		}
	}
	return res, timestamp, nil
}

func (c *H3cTelemetryMDT) Address() net.Addr {
	return c.listener.Addr()
}

// Stop listener and cleanup
func (c *H3cTelemetryMDT) Stop() {
	if c.grpcServer != nil {
		// Stop server and terminate all running dialout routines
		c.grpcServer.Stop()
	}
	if c.listener != nil {
		c.listener.Close()
	}
	c.wg.Wait()
}

const sampleConfig = `
 ## Address and port to host telemetry listener
 service_address = "10.0.0.1:57000"

 ## Enable TLS; 
 # tls_cert = "/etc/telegraf/cert.pem"
 # tls_key = "/etc/telegraf/key.pem"

 ## Enable TLS client authentication and define allowed CA certificates; grpc
 ##  transport only.
 # tls_allowed_cacerts = ["/etc/telegraf/clientca.pem"]

 ## Define (for certain nested telemetry measurements with embedded tags) which fields are tags

 ## Define aliases to map telemetry encoding paths to simple measurement names
`

// SampleConfig of plugin
func (c *H3cTelemetryMDT) SampleConfig() string {
	return sampleConfig
}

// Description of plugin
func (c *H3cTelemetryMDT) Description() string {
	return "H3C Telemetry For H3C Router"
}

// Gather plugin measurements (unused)
func (c *H3cTelemetryMDT) Gather(_ telegraf.Accumulator) error {
	return nil
}

func init() {
	inputs.Add("h3c_telemetry_mdt", func() telegraf.Input {
		return &H3cTelemetryMDT{}
	})
}

func (c *H3cTelemetryMDT) stop() {
	log.SetOutput(os.Stderr)
	log.Printf("I! telegraf stopped because error.")
	os.Exit(1)
}
