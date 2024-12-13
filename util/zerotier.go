package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const zerotierMemberAPI = "https://api.zerotier.com/api/v1/network/%s/member/%s"
const zerotierMemberListAPI = "https://api.zerotier.com/api/v1/network/%s/member"

type ZerotierMemberList []ZerotierMember

type ZerotierMember struct {
	ID                  string         `json:"id"`
	Type                string         `json:"type"`
	Clock               int64          `json:"clock"`
	NetworkID           string         `json:"networkId"`
	NodeID              string         `json:"nodeId"`
	ControllerID        string         `json:"controllerId"`
	Hidden              bool           `json:"hidden"`
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	Config              ZerotierConfig `json:"config"`
	LastOnline          int64          `json:"lastOnline"`
	LastSeen            int64          `json:"lastSeen"`
	PhysicalAddress     string         `json:"physicalAddress"`
	PhysicalLocation    interface{}    `json:"physicalLocation"`
	ClientVersion       string         `json:"clientVersion"`
	ProtocolVersion     int            `json:"protocolVersion"`
	SupportsRulesEngine bool           `json:"supportsRulesEngine"`
}

type ZerotierConfig struct {
	ActiveBridge         bool          `json:"activeBridge"`
	Address              string        `json:"address"`
	Authorized           bool          `json:"authorized"`
	Capabilities         []interface{} `json:"capabilities"`
	CreationTime         int64         `json:"creationTime"`
	ID                   string        `json:"id"`
	Identity             string        `json:"identity"`
	IPAssignments        []string      `json:"ipAssignments"`
	LastAuthorizedTime   int64         `json:"lastAuthorizedTime"`
	LastDeauthorizedTime int           `json:"lastDeauthorizedTime"`
	NoAutoAssignIps      bool          `json:"noAutoAssignIps"`
	Nwid                 string        `json:"nwid"`
	Objtype              string        `json:"objtype"`
	RemoteTraceLevel     int           `json:"remoteTraceLevel"`
	RemoteTraceTarget    string        `json:"remoteTraceTarget"`
	Revision             int           `json:"revision"`
	Tags                 []interface{} `json:"tags"`
	VMajor               int           `json:"vMajor"`
	VMinor               int           `json:"vMinor"`
	VRev                 int           `json:"vRev"`
	VProto               int           `json:"vProto"`
	SsoExempt            bool          `json:"ssoExempt"`
}

// GetNetworkMemberList 处理HTTP结果，返回序列化的json
func GetNetworkMemberList(zerotierStr string) (*ZerotierMemberList, error) {
	zArray := strings.Split(zerotierStr, ",")
	if len(zArray) < 2 {
		return nil, fmt.Errorf("zerotier config error")
	}

	result := new(ZerotierMemberList)

	token, network := zArray[0], zArray[1]
	url := fmt.Sprintf(zerotierMemberListAPI, network)
	req, err := http.NewRequest(
		"GET",
		url,
		bytes.NewBuffer([]byte{}),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)

	client := CreateHTTPClient()
	resp, err := client.Do(req)
	err = GetHTTPResponse(resp, err, result)

	return result, nil
}

// GetNetworkMemberList 处理HTTP结果，返回序列化的json
func GetNetworkMember(zerotierStr string) (*ZerotierMember, error) {
	zArray := strings.Split(zerotierStr, ",")
	if len(zArray) < 3 {
		return nil, fmt.Errorf("zerotier config error")
	}

	result := new(ZerotierMember)

	token, network, nodeId := zArray[0], zArray[1], zArray[2]
	url := fmt.Sprintf(zerotierMemberAPI, network, nodeId)
	req, err := http.NewRequest(
		"GET",
		url,
		bytes.NewBuffer([]byte{}),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)

	client := CreateHTTPClient()
	resp, err := client.Do(req)
	err = GetHTTPResponse(resp, err, result)

	return result, nil
}

// DeepCopy 深拷贝
func DeepCopy[T any](obj T) *T {
	jsonStr, _ := json.Marshal(obj)
	var newObj = new(T)
	_ = json.Unmarshal(jsonStr, newObj)
	return newObj
}
