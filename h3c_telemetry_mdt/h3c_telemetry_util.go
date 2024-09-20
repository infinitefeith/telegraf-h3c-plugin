package h3c_telemetry_mdt

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	KeySeperator        = "." // A nested delimiter for Tag or Field
	MsgTimeStampKeyName = "timestamp"
	JsonMsgKeyName      = "data_str"
	GpbMsgKeyName       = "data_gpb"
	RowKeyName          = "row"
	TimeFormat          = "2006-01-02 15:04:05" // time.RFC3339
	SensorPathKey       = "sensor_path"
)

// Convert the Proto Message to a Map
func protoMsgToMap(protoMsg proto.Message) (map[string]interface{}, error) {
	// trans proto.Message into map[string]interface{}]
	protoToJson := protojson.MarshalOptions{
		UseEnumNumbers: false,
		UseProtoNames:  true,
	}
	pb, errToJson := protoToJson.Marshal(protoMsg)
	if errToJson != nil {
		return nil, fmt.Errorf("[grpc parser] proto message decode to json: %v", errToJson)
	}
	var msgMap map[string]interface{}
	errToMap := json.Unmarshal(pb, &msgMap)
	if errToMap != nil {
		return nil, fmt.Errorf("[grpc parser] proto message decodec to json: %v", errToMap)
	}
	return msgMap, nil
}

type KVStruct struct {
	Fields map[string]interface{}
}

func (kv *KVStruct) FullFlattenStruct(fieldname string,
	v interface{},
	convertString bool,
	convertBool bool) error {
	if kv.Fields == nil {
		kv.Fields = make(map[string]interface{})
	}
	switch t := v.(type) {
	case map[string]interface{}:
		for k, v := range t {
			fieldKey := k
			if fieldname != "" {
				fieldKey = fieldname + KeySeperator + fieldKey
			}
			err := kv.FullFlattenStruct(fieldKey, v, convertString, convertBool)
			if err != nil {
				return err
			}
		}
	case []interface{}:
		for i, v := range t {
			fieldKey := strconv.Itoa(i)
			if fieldname != "" {
				fieldKey = fieldname + KeySeperator + fieldKey
			}
			err := kv.FullFlattenStruct(fieldKey, v, convertString, convertBool)
			if err != nil {
				return nil
			}
		}
	case float64:
		kv.Fields[fieldname] = v.(float64)
	case float32:
		kv.Fields[fieldname] = v.(float32)
	case uint64:
		kv.Fields[fieldname] = v.(uint64)
	case uint32:
		kv.Fields[fieldname] = v.(uint32)
	case int64:
		kv.Fields[fieldname] = v.(int64)
	case int32:
		kv.Fields[fieldname] = v.(int32)
	case string:
		if convertString {
			kv.Fields[fieldname] = v.(string)
		} else {
			return nil
		}
	case bool:
		if convertBool {
			kv.Fields[fieldname] = v.(bool)
		} else {
			return nil
		}
	case nil:
		return nil
	default:
		return fmt.Errorf("key Value Flattener : got unexpected type %T with value %v (%s)", t, t, fieldname)

	}
	return nil
}

// check if the data is num and return as
func convertToNum(str string) (bool, int64) {
	num, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return false, 0
	} else {
		return true, num
	}
}

// timestamp transfer into time
// ten bit timestamp with second, 13 bit timestamp with second
// time.Unix(s,ns)
func calTimeByStamp(v interface{}) (time.Time, string, error) {
	var sec int64
	var nsec int64
	switch v.(type) {
	case float64:
		vInFloat64 := v.(float64)
		if vInFloat64 < math.Pow10(11) {
			sec = int64(vInFloat64)
			nsec = 0
		}
		if vInFloat64 > math.Pow10(12) {
			sec = int64(vInFloat64 / 1000)
			nsec = (int64(vInFloat64) % 1000) * 1000 * 1000
		}
	case int64:
		vInInt64 := v.(int64)
		if float64(vInInt64) < math.Pow10(11) {
			sec = vInInt64
			nsec = 0
		}
		if float64(vInInt64) > math.Pow10(12) {
			sec = vInInt64 / 1000
			nsec = (vInInt64 % 1000) * 1000 * 1000
		}
	case uint64:
		vInUint64 := v.(uint64)
		if float64(vInUint64) < math.Pow10(11) {
			sec = int64(vInUint64)
			nsec = 0
		}
		if float64(vInUint64) > math.Pow10(12) {
			sec = int64(vInUint64 / 1000)
			nsec = int64((vInUint64 % 1000) * 1000 * 1000)
		}
	case string:
		if strings.Index(v.(string), ":") > -1 {
			return time.Time{}, v.(string), nil
		}
		timeInNum, errToNum := strconv.ParseUint(v.(string), 10, 64)
		if errToNum != nil {
			log.Printf("E! [grpc.parser.calTimeByStamp] v: %t , error : %v", v, errToNum)
		} else {
			if float64(timeInNum) < math.Pow10(11) {
				sec = int64(timeInNum)
				nsec = 0
			}
			if float64(timeInNum) > math.Pow10(12) {
				sec = int64(timeInNum / 1000)
				nsec = int64((timeInNum % 1000) * 1000 * 1000)
			}
		}
	}

	if sec == 0 {
		return time.Time{}, "", errors.New("calculate error")
	}
	time := time.Unix(sec, nsec)
	return time, time.Format(TimeFormat), nil
}

// struct reflect.Type set
type ProtoTypes struct {
	typeSet []reflect.Type // Array can not repeat
}

// the key struct in map
type PathKey struct {
	ProtoPath string
	Version   string
}

// proto key value
type ProtoOrganizeType int

const (
	// proto type mark represents h3c, put them under one proto, and encaps them into type
	PROTO_H3C_TYPE  = 0
	PROTO_IETF_TYPE = 1
)

const (
	DEFAULT_VERSION = "1.0"
)

// get all ProtoPath
func GetProtoPaths() []*PathKey {
	paths := make([]*PathKey, len(pathTypeMap))
	i := 0
	for key := range pathTypeMap {
		path := key
		paths[i] = &path
		i++
	}
	return paths
}

// get reflect.Type set pointer by protokey
func GetProtoTypeSetByKey(p *PathKey) *ProtoTypes {
	set := &ProtoTypes{
		typeSet: pathTypeMap[*p],
	}
	if set.typeSet == nil {
		return nil
	}
	return set
}

// get protoPath with protoPath and version
func GetTypeByProtoPath(protoPath string, version string) (proto.Message, error) {
	if version == "" {
		version = DEFAULT_VERSION
	}
	mapping := GetProtoTypeSetByKey(
		&PathKey{
			ProtoPath: protoPath,
			Version:   DEFAULT_VERSION})
	if mapping == nil {
		return nil, fmt.Errorf("the proto type is nil , protoPath is %s", protoPath)
	}
	typeInMap := mapping.GetTypesByProtoOrg(PROTO_H3C_TYPE) // using reflect
	elem := typeInMap.Elem()
	reflectType := reflect.New(elem)
	contentType := reflectType.Interface().(proto.Message)
	return contentType, nil
}

// get proto type by proto
func (p *ProtoTypes) GetTypesByProtoOrg(orgType ProtoOrganizeType) reflect.Type {
	varTypes := p.typeSet
	if varTypes == nil {
		return nil
	}
	if len(varTypes) > int(orgType) {
		return varTypes[orgType]
	}
	return nil
}

// one map key: protoPath + version, value : reflect[]
var pathTypeMap = map[PathKey][]reflect.Type{

	//PathKey{ProtoPath: "huawei_debug.Debug", Version: "1.0"}: []reflect.Type{reflect.TypeOf((*huawei_debug.Debug)(nil))},
}
